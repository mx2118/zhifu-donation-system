package utils

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/zhifu/donation-rank/models"
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

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			LogLevel: logLevel,
		},
	)

	// 连接数据库
	var err error
	log.Printf("Attempting to connect to database: %s:%d/%s", host, port, dbname)
	log.Printf("DSN: %s", dsn)
	
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})

	if err != nil {
		log.Printf("Failed to connect to database: %v", err)
		log.Printf("Connection details: host=%s, port=%d, user=%s, dbname=%s", host, port, user, dbname)
		return err
	}
	
	log.Printf("Database connection successful!")

	// 配置数据库连接池
	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Failed to get database: %v", err)
		return err
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(15)           // 最大空闲连接数
	sqlDB.SetMaxOpenConns(120)          // 最大打开连接数
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接最大生命周期
	sqlDB.SetConnMaxIdleTime(30 * time.Minute) // 连接最大空闲时间

	// 跳过数据库迁移，根据用户要求
	return nil
}

// MigrateDatabase 手动执行数据库迁移
func MigrateDatabase() {
	migrateDatabase()
}

// migrateDatabase 执行数据库迁移
func migrateDatabase() {
	// 创建必要的表
	log.Println("Starting database migration...")
	DB.AutoMigrate(
		&models.Donation{},
		&models.PaymentConfig{},
		&models.WechatUser{},
		&models.AlipayUser{},
		&models.Category{},
	)
	log.Println("Database migration completed successfully!")
}
