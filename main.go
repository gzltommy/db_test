package main

import "C"
import (
	"db_test/mysql"
	"db_test/redis"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"math"
	"math/rand"
	"time"
)

func main() {
	defer mysql.Close()
	// 获取用户数据

	total, list, except := FindPopularSpace(1, 10, []string{"0xe183ea3871685f340ffdb8b784a20d3c9d3511be"}, nil)

	buf, _ := json.Marshal(list)
	fmt.Println("+++++++++++++++", total, string(buf), len(except))

}

const (
	MaxSystemSpaceId = 10000
	SpaceUserRank    = "space_user_rank"
)

// FindPopularSpace
// （1）置顶 space；
// （2）【在线人数】：前 5 个 Space 按照在线人数来排序；
// （3）【艺术馆】：按艺术馆 NFT 挂载数量，从前 10 名中随机 5 个；
// （4）【个人馆】：随机 25 个馆
func FindPopularSpace(page, limit int, artContracts []string, exceptSpaceIds []int64) (int64, []SpaceBase, []int64) {
	if len(artContracts) == 0 {
		return 0, nil, nil
	}
	var (
		totalCount int64
		raws       []SpaceBase
	)

	// 置顶的 Space（最高优先级）
	// 所有的 Space 只保留英文 Space
	ret := mysql.DB().Model(&Space{}).
		Joins("as a left join users as b on a.user_id = b.id").
		Select("a.*,b.head_img as creator_image,length(b.head_img_nft)>0 as creator_nft_image,b.user_name as creator_name").
		Where("a.status=?", 1).
		//Where("length(a.name)=char_length(a.name)").
		//Where("length(a.bio)=char_length(a.bio)").
		Where("a.china_word=?", 0).
		Where("a.id >= ?", MaxSystemSpaceId)

	//ret.Count(&totalCount)

	// 1.置顶数据
	var toppingCount int64
	ret.Where("a.topping>?", 0).
		Count(&toppingCount) // 置顶的条数

	totalCount = toppingCount + 5 + 5 + 25

	// 置顶里面还有数据
	if int64((page-1)*limit) < toppingCount {
		ret.Where("a.topping>?", 0).
			Order("a.topping asc").
			Offset((page - 1) * limit).
			Limit(limit).
			Scan(&raws)
	}

	// topping 阶段取的数据已经足够；或者所有的数据已经取完了
	curToppingNum := len(raws)
	if curToppingNum == limit || (curToppingNum+(page-1)*limit) >= int(totalCount) {
		return totalCount, raws, exceptSpaceIds
	}

	// 非置顶数据（置顶的数据不够）
	ret = ret.Where("a.topping=?", 0)
	nPage := page - int(toppingCount)/limit
	nOffset := (nPage-1)*limit - int(toppingCount)%limit
	if nOffset < 0 {
		nOffset = 0
	}
	nLimit := limit - curToppingNum

	//2.【在线人数】：前 5 个 Space 按照在线人数来排序
	var (
		stage2Num      = 5
		sStage2Num     = stage2Num - ((page-1)*limit + curToppingNum - int(toppingCount)) // 在线人数剩余取用条数
		stage2Raws     []SpaceBase
		curStage2Count = 0
	)
	// 未跳过 stage 2 阶段(因为房间在线人数实时变动，将这阶段的数据也放进排除id处理逻辑)
	if sStage2Num > 0 {
		var (
			sysSpaceCount   = 20
			includeSpaceIds []int64
			needNum         = int(math.Min(float64(sStage2Num), float64(nLimit)))
			actualNum       = 0
		)
		spaceUserRank, _ := redis.ZrevRangeWithScore(SpaceUserRank, 0, sysSpaceCount+int(toppingCount))
		for i := 0; i < len(spaceUserRank); i += 2 {
			spaceId := spaceUserRank[i]
			//userNum := spaceUserRank[i+1]
			if spaceId < MaxSystemSpaceId {
				continue
			}
			except := false
			for _, id := range exceptSpaceIds {
				if spaceId == id {
					except = true
					break
				}
			}
			if except {
				continue
			}
			actualNum += 1
			includeSpaceIds = append(includeSpaceIds, spaceId)
			if actualNum >= needNum {
				break
			}
		}

		// 需要取用的数据量 > 该阶段剩余的名额量：这阶段取一部分，下个阶段取一部分
		stage2Raws = func(includeSpaceIds, exceptSpaceIds []int64, needNum int) []SpaceBase {
			var rRaws []SpaceBase
			ret1 := ret.Session(&gorm.Session{})
			if len(includeSpaceIds) > 0 {
				ret1 = ret1.Where("a.id in (?)", includeSpaceIds)
			}
			if len(exceptSpaceIds) > 0 {
				ret1 = ret1.Where("a.id not in(?)", exceptSpaceIds)
			}
			ret1.Scan(&rRaws)

			// redis 里面只有房间有人的才有数据
			if n := needNum - len(rRaws); n > 0 {
				var rRaws2 []SpaceBase
				ret2 := ret.Session(&gorm.Session{})
				if len(includeSpaceIds) > 0 {
					ret2 = ret2.Where("a.id not in (?)", includeSpaceIds)
				}
				if len(exceptSpaceIds) > 0 {
					ret2 = ret2.Where("a.id not in(?)", exceptSpaceIds)
				}
				ret2.Limit(n).Scan(&rRaws2)
				rRaws = append(rRaws, rRaws2...)
			}
			return rRaws
		}(includeSpaceIds, exceptSpaceIds, needNum)

		for _, v := range stage2Raws {
			exceptSpaceIds = append(exceptSpaceIds, v.ID)
		}
		raws = append(raws, stage2Raws...)
		curStage2Count = len(stage2Raws)
		// 需要取用的数据量 <= 该阶段剩余的名额量：都在该阶段取
		if nLimit <= sStage2Num {
			return totalCount, raws, exceptSpaceIds
		} else {
			// 需要取用的数据量 > 该阶段剩余的名额量：这阶段取一部分，下个阶段取一部分

			// 已经没有剩余的数据可取用了，直接返回
			if curStage2Count < sStage2Num {
				return totalCount, raws, exceptSpaceIds
			}

			// 取用下一阶段的数据
			nOffset = 0
			nLimit = nLimit - len(stage2Raws)
		}
	} else {
		// 已跳过优先显示的 5 条的阶段
		nPage = page - (int(toppingCount)+stage2Num)/limit
		nOffset = (nPage-1)*limit - (int(toppingCount)+stage2Num)%limit
		if nOffset < 0 {
			nOffset = 0
		}
	}

	//3.【艺术馆】：按艺术馆 NFT 挂载数量，从前 10 名中随机 5 个；
	var (
		stage3BaseNum  = 10
		stage3Num      = 5
		sStage3Num     = stage3Num - ((page-1)*limit + curToppingNum + curStage2Count - int(toppingCount) - stage2Num) // 艺术馆 nft 挂载排名
		stage3Raws     []SpaceBase
		curStage3Count = 0
	)

	// 未跳过 stage 3 阶段（这阶段有随机逻辑，将这阶段的数据也放进排除id处理逻辑）
	if sStage3Num > 0 {
		var (
			randNum = stage3BaseNum - (stage3Num - sStage3Num)
			needNum = int(math.Min(float64(sStage3Num), float64(nLimit)))
		)
		if needNum > randNum {
			needNum = randNum
		}
		stage3Raws = func(nOffset, randNum, needNum int, artContracts []string, exceptSpaceIds []int64) []SpaceBase {
			var rRaws []SpaceBase
			ret1 := ret.Session(&gorm.Session{})
			ret1 = ret1.Where("a.contract in (?)", artContracts)
			if len(exceptSpaceIds) > 0 {
				ret1 = ret1.Where("a.id not in(?)", exceptSpaceIds)
			}
			ret1.Order("a.asset_count desc").
				Offset(nOffset).
				Limit(randNum).
				Scan(&rRaws)
			// 打乱顺序
			rand.Seed(time.Now().UnixNano())
			rankSpaceIdLen := len(rRaws)
			for i := 0; i < rankSpaceIdLen; i++ {
				index := rand.Intn(rankSpaceIdLen)
				rRaws[i], rRaws[index] = rRaws[index], rRaws[i]
			}
			return raws[:needNum]
		}(0, randNum, needNum, artContracts, exceptSpaceIds)

		for _, v := range stage3Raws {
			exceptSpaceIds = append(exceptSpaceIds, v.ID)
		}
		curStage3Count = len(stage3Raws)
		raws = append(raws, stage3Raws...)

		// b.需要取用的数据量 <= 该阶段剩余的名额量：都在该阶段取
		if nLimit <= sStage3Num {
			return totalCount, raws, exceptSpaceIds
		} else {
			// b.需要取用的数据量 > 该阶段剩余的名额量：这阶段取一部分，下个阶段取一部分
			// 取用下一阶段的数据
			nOffset = 0
			nLimit = nLimit - sStage3Num
		}
	} else {
		// 已跳过 stage3 的阶段
		nPage = page - (int(toppingCount)+stage2Num+stage3Num)/limit
		nOffset = (nPage-1)*limit - (int(toppingCount)+stage2Num+stage3Num)%limit
		if nOffset < 0 {
			nOffset = 0
		}
	}

	//（4）【个人馆】：随机 25 个馆
	var (
		stage4Num  = 25
		sStage4Num = stage4Num - ((page-1)*limit + curToppingNum + curStage2Count + curStage3Count - int(toppingCount) - stage2Num - stage3Num)
		stage4Raws []SpaceBase
	)
	if len(exceptSpaceIds) > 0 {
		ret = ret.Where("id not in(?)", exceptSpaceIds)
	}
	ret.Where("a.contract not in (?)", artContracts).
		Order("rand()").
		Offset(nOffset).
		Limit(sStage4Num).
		Scan(&stage4Raws)

	raws = append(raws, stage4Raws...)
	for _, v := range stage4Raws {
		exceptSpaceIds = append(exceptSpaceIds, v.ID)
	}
	return totalCount, raws, exceptSpaceIds
}

type SpaceBase struct {
	Space
	Image           string                `json:"image"`
	AnimationUrl    string                `json:"animation_url"`
	Attributes      string                `json:"attributes"`
	CurrPlayer      int                   `json:"curr_player"`
	SupportVoice    int                   `json:"support_voice"`
	IsLike          bool                  `json:"is_like"`
	LikeCount       int64                 `json:"like_count" gorm:"-"`
	LastEnterUsers  []SpaceEnterUserModle `json:"last_enter_users" gorm:"-"`
	CreatorName     string                `json:"creator_name"`
	CreatorImage    string                `json:"creator_image"`
	CreatorNftImage int                   `json:"creator_nft_image"`
	Views           int64                 `json:"views" gorm:"-"`
}

type Space struct {
	ID           int64  `gorm:"primary_key"`
	UserId       int64  `json:"user_id"`
	Address      string `json:"address"`
	Name         string `json:"name"`
	Bio          string `json:"bio"`
	Quality      int64  `json:"quality"`
	PlayerLimit  int32  `json:"player_limit"`
	ShareLimit   int32  `json:"share_limit"`
	ChainId      int    `json:"chain_id"`
	Contract     string `json:"contract"`
	TokenId      string `json:"token_id"`
	TxHash       string `json:"tx_hash"`
	TV           string `json:"tv"`
	NFT          string `json:"nft"`
	Furniture    string `json:"furniture"`
	Status       int    `json:"status"`
	SpaceType    int    `json:"space_type"`
	CreateHash   string `json:"create_hash"`
	DeleteHash   string `json:"delete_hash"`
	Duraction    int64  `json:"duraction"`
	ExpireTime   int64  `json:"expire_time"`
	CreateTime   int64  `json:"create_time"`
	Topping      int    `json:"topping"`
	Reason       string `json:"reason"`
	ChinaWord    int    `json:"-"`
	SupportVoice int    `json:"support_voice"`
	Config       string `json:"config"`
	AssetCount   int    `json:"asset_count"`
}

type SpaceEnterUserModle struct {
	UserId     int64  `json:"user_id"`
	HeadImg    string `json:"head_img"`
	HeadImgNFT string `json:"head_img_nft"`
}
