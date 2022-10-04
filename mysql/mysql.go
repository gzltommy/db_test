// 定义一个工具包，用来管理gorm数据库连接池的初始化工作。
package mysql

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 定义全局的 db 对象，我们执行数据库操作主要通过他实现。
var db *gorm.DB

// 包初始化函数，golang 特性，每个包初始化的时候会自动执行init 函数，这里用来初始化 gorm。
func init() {
	//配置MySQL连接参数
	username := "root"       //账号
	password := "123456"     //密码
	host := "192.168.24.133" //数据库地址，可以是 Ip 或者域名
	Dbname := "test2"        //数据库名
	port := 3306             //数据库端口

	timeout := "10s" //连接超时，10秒

	//拼接下dsn参数, dsn格式可以参考上面的语法，这里使用Sprintf动态拼接dsn参数，因为一般数据库连接参数，我们都是保存在配置文件里面，需要从配置文件加载参数，然后拼接dsn。
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8&parseTime=True&loc=Local&timeout=%s", username, password, host, port, Dbname, timeout)

	//连接 MYSQL, 获得 tools 类型实例，用于后面的数据库读写操作。
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic("连接数据库失败, error=" + err.Error())
	}

	sqlDB, _ := db.DB()

	//设置数据库连接池参数
	sqlDB.SetMaxOpenConns(100) //设置数据库连接池最大连接数
	sqlDB.SetMaxIdleConns(20)  //连接池最大允许的空闲连接数，如果没有sql任务需要执行的连接数大于20，超过的连接会被连接池关闭。
}

// DB 获取 gorm db 对象，其他包需要执行数据库查询的时候，只要通过 tools.getDB() 获取 db 对象即可。
// 不用担心协程并发使用同样的 db 对象会共用同一个连接，db 对象在调用他的方法的时候会从数据库连接池中获取新的连接
func DB() *gorm.DB {
	return db
}

func Close() error {
	if db != nil {
		idb, err := db.DB()
		if err == nil {
			idb.Close()
		}
	}
	fmt.Println("close mysql connect")
	return nil
}
