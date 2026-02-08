package utils

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDatabase(host, user, password, dbname string, port int) error {
	// 构建DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, password, host, port, dbname)

	// 配置日志
	// 根据环境调整日志级别
	logLevel := logger.Info
	if os.Getenv("GO_ENV") == "production" {
		logLevel = logger.Error // 生产环境只记录错误
	}

	// 使用默认日志配置
	newLogger := logger.Default.LogMode(logLevel)

	// 连接数据库
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})

	if err != nil {
		return err
	}

	// 配置数据库连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(30)           // 增加最大空闲连接数，提高并发处理能力
	sqlDB.SetMaxOpenConns(300)          // 增加最大打开连接数，适应高并发场景
	sqlDB.SetConnMaxLifetime(5 * time.Minute) // 连接最大生命周期，避免使用过期连接
	sqlDB.SetConnMaxIdleTime(1 * time.Minute) // 连接最大空闲时间，释放不必要的连接
	
	// 验证连接池配置
	log.Printf("Database connection pool configured: MaxIdle=%d, MaxOpen=%d, MaxLifetime=%s, MaxIdleTime=%s",
		30, 300, 5*time.Minute, 1*time.Minute)

	return nil
}
