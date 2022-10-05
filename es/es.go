package es

import (
	"github.com/olivere/elastic/v7"
	"time"
)

var es *elastic.Client

// 这里一定要使用ES7.x版本，不然会导致语法不兼容
func init() {
	url := "http://192.168.13.24:9200"
	username := "elastic"
	password := "123456"

	var err error
	es, err = elastic.NewClient(
		elastic.SetURL(url),
		//docker
		elastic.SetSniff(false),
		elastic.SetBasicAuth(username, password),
		// 设置监控检查时间间隔
		elastic.SetHealthcheckInterval(10*time.Second),
		// 开启健康检查
		elastic.SetHealthcheck(true),
		// 重试策略
		elastic.SetRetrier(elastic.NewBackoffRetrier(elastic.NewSimpleBackoff(500, 500, 500, 500, 500))),
	)

	if err != nil {
		panic(err)
	}
	es.Start()
}
