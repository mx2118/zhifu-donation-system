package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gzip "github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/zhifu/donation-rank/models"
	"github.com/zhifu/donation-rank/routes"
	"github.com/zhifu/donation-rank/services"
	"github.com/zhifu/donation-rank/utils"
)

func main() {
	// 获取当前执行文件的目录
	execDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatalf("Failed to get exec dir: %v", err)
	}

	// 获取当前工作目录
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working dir: %v", err)
	}

	log.Printf("Exec dir: %s", execDir)
	log.Printf("Working dir: %s", workDir)

	// 优先从当前工作目录加载配置文件
	viper.SetConfigFile("config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		// 如果当前目录找不到，再尝试从执行文件目录查找
		viper.SetConfigFile(filepath.Join(execDir, "config.yaml"))
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalf("Failed to read config: %v", err)
		}
	}

	// 初始化数据库
	dbConnected := false
	if err := utils.InitDatabase(
		viper.GetString("mysql.host"),
		viper.GetString("mysql.user"),
		viper.GetString("mysql.password"),
		viper.GetString("mysql.dbname"),
		viper.GetInt("mysql.port"),
	); err != nil {
		log.Printf("Warning: Database connection failed: %v", err)
		log.Printf("Server will start without database connection, some features may be limited")
		dbConnected = false
	} else {
		dbConnected = true
		log.Printf("Database connected successfully")
	}

	// 初始化主支付服务配置
	var paymentService *services.PaymentService

	// 从数据库加载支付配置
	loadPaymentConfig := func() services.ShouqianbaConfig {
		// 默认配置
		defaultConfig := services.ShouqianbaConfig{
			VendorSN:   "default",
			VendorKey:  "default",
			AppID:      "default",
			DeviceID:   "default",
			APIURL:     "http://api.example.com",
			GatewayURL: "http://gateway.example.com",
		}

		if !dbConnected {
			log.Printf("Using default payment service configuration due to database connection failure")
			return defaultConfig
		}

		// 对id=6、id=1和id=2的支付配置分别进行签到
		configsToSignIn := []int{6, 1, 2}
		var mainConfig models.PaymentConfig
		var mainConfigFound bool

		for _, configID := range configsToSignIn {
			var config models.PaymentConfig
			if err := utils.DB.Where("id = ?", configID).First(&config).Error; err != nil {
				log.Printf("Config with id=%d not found: %v", configID, err)
				continue
			}

			log.Printf("Processing payment config (id=%d): %s", configID, config.TerminalSN)

			// 为当前配置创建独立的支付服务并签到
			configService := services.NewPaymentService(services.ShouqianbaConfig{
				VendorSN:         config.VendorSN,
				VendorKey:        config.VendorKey,
				AppID:            config.AppID,
				TerminalSN:       config.TerminalSN,
				TerminalKey:      config.TerminalKey,
				DeviceID:         config.DeviceID,
				MerchantID:       config.MerchantID,
				StoreID:          config.StoreID,
				StoreName:        config.StoreName,
				APIURL:           config.APIURL,
				GatewayURL:       config.GatewayURL,
				WechatAppID:      config.WechatAppID,
				WechatAppSecret:  config.WechatAppSecret,
				AlipayAppID:      config.AlipayAppID,
				AlipayPublicKey:  config.AlipayPublicKey,
				AlipayPrivateKey: config.AlipayPrivateKey,
			})

			// 终端签到，更新terminal_key
			if err := configService.SignIn(); err != nil {
				log.Printf("Terminal sign-in failed for config id=%d: %v", configID, err)
			} else {
				log.Printf("Terminal sign-in successful for config id=%d: %s", configID, config.TerminalSN)
			}

			// 设置主配置（优先使用id=6）
			if configID == 6 || !mainConfigFound {
				mainConfig = config
				mainConfigFound = true
			}
		}

		// 如果没有找到配置，尝试使用is_active=true的配置
		if !mainConfigFound {
			if err := utils.DB.Where("is_active = ?", true).First(&mainConfig).Error; err != nil {
				log.Printf("Warning: No payment config found in database. Using default configuration.")
				return defaultConfig
			} else {
				log.Printf("Loaded payment config from database (active): %s", mainConfig.TerminalSN)
				mainConfigFound = true
			}
		}

		// 使用找到的配置
		return services.ShouqianbaConfig{
			VendorSN:         mainConfig.VendorSN,
			VendorKey:        mainConfig.VendorKey,
			AppID:            mainConfig.AppID,
			TerminalSN:       mainConfig.TerminalSN,
			TerminalKey:      mainConfig.TerminalKey,
			DeviceID:         mainConfig.DeviceID,
			MerchantID:       mainConfig.MerchantID,
			StoreID:          mainConfig.StoreID,
			StoreName:        mainConfig.StoreName,
			APIURL:           mainConfig.APIURL,
			GatewayURL:       mainConfig.GatewayURL,
			WechatAppID:      mainConfig.WechatAppID,
			WechatAppSecret:  mainConfig.WechatAppSecret,
			AlipayAppID:      mainConfig.AlipayAppID,
			AlipayPublicKey:  mainConfig.AlipayPublicKey,
			AlipayPrivateKey: mainConfig.AlipayPrivateKey,
		}
	}

	// 加载配置并创建支付服务
	paymentConfig := loadPaymentConfig()
	paymentService = services.NewPaymentService(paymentConfig)

	// 设置 GIN 为生产模式
	gin.SetMode(gin.ReleaseMode)

	// 初始化路由，使用自定义中间件
	router := gin.New()

	// 设置可信代理，消除安全警告
	router.SetTrustedProxies([]string{"127.0.0.1"}) // 替换为你的代理IP

	// 添加必要的中间件
	router.Use(gin.Recovery())

	// 添加gzip压缩中间件
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	// 添加安全头部和CORS中间件
	router.Use(func(c *gin.Context) {
		// 安全头部
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")

		// CORS配置
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理OPTIONS请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// 初始化 API 路由
	apiRoutes := routes.NewAPIRoutes(paymentService)
	// 使用当前工作目录作为baseDir，确保能找到静态文件
	apiRoutes.SetupRoutes(router, workDir)

	// 配置 HTTP 服务器
	port := viper.GetInt("server.port")
	addr := fmt.Sprintf(":%d", port) // 监听所有网络接口

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Server running on http://localhost%s", addr)
	log.Printf("Server mode: %s", gin.Mode())

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}
