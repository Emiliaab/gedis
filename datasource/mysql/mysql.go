package mysql

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func New() *gorm.DB {
	//建立数据库连接,数据库名:数据库密码@...
	dsn := "root:root@tcp(127.0.0.1:3306)/gedis?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	//处理错误
	if err != nil {
		//控制台打印错误日志
		panic("数据库连接失败!")
	}
	return db
}

// 表的结构
type Data struct {
	Key   string `gorm:"column:gedis_key"`
	Value string `gorm:"column:gedis_value"`
}
