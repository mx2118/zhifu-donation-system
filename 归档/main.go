package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
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

	// 优先从当前工作目录加载配置文件
	viper.SetConfigFile("config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		// 如果当前目录找不到，再尝试从执行文件目录查找
		viper.SetConfigFile(filepath.Join(execDir, "config.yaml"))
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalf("Failed to read config: %v", err)
		}
	}

	// 初始化缓存
	utils.InitCache()
	log.Println("Cache manager initialized successfully")
	
	// 初始化数据库
	dbConnected := utils.InitDatabase(
		viper.GetString("mysql.host"),
		viper.GetString("mysql.user"),
		viper.GetString("mysql.password"),
		viper.GetString("mysql.dbname"),
		viper.GetInt("mysql.port"),
	) == nil

	if dbConnected {
		log.Printf("Database connected successfully")
	} else {
		log.Printf("Warning: Database connection failed, some features may be limited")
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
			return defaultConfig
		}

		// 优先使用id=6的配置
		var mainConfig models.PaymentConfig
		if err := utils.DB.Where("id = ?", 6).First(&mainConfig).Error; err != nil {
			// 尝试使用id=1的配置
			if err := utils.DB.Where("id = ?", 1).First(&mainConfig).Error; err != nil {
				// 尝试使用id=2的配置
				if err := utils.DB.Where("id = ?", 2).First(&mainConfig).Error; err != nil {
					// 尝试使用is_active=true的配置
					if err := utils.DB.Where("is_active = ?", true).First(&mainConfig).Error; err != nil {
						return defaultConfig
					}
				}
			}
		}

		// 为选中的配置创建支付服务并签到
		configService := services.NewPaymentService(services.ShouqianbaConfig{
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
		})

		// 终端签到，更新terminal_key
		if err := configService.SignIn(); err != nil {
			log.Printf("Terminal sign-in failed: %v", err)
		} else {
			log.Printf("Terminal sign-in successful: %s", mainConfig.TerminalSN)
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

	// 初始化 API 路由
	apiRoutes := routes.NewAPIRoutes(paymentService)

	// 创建fasthttp请求处理器
	handler := func(ctx *fasthttp.RequestCtx) {
		method := string(ctx.Method())

		// 添加安全头部
		ctx.Response.Header.Set("X-Content-Type-Options", "nosniff")
		ctx.Response.Header.Set("X-Frame-Options", "DENY")
		ctx.Response.Header.Set("X-XSS-Protection", "1; mode=block")

		// CORS配置
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理OPTIONS请求
		if method == "OPTIONS" {
			ctx.SetStatusCode(fasthttp.StatusNoContent)
			return
		}

		// 处理API路由
		apiRoutes.HandleRequest(ctx, workDir)
	}

	// 配置 HTTP 服务器
	port := viper.GetInt("server.port")
	addr := fmt.Sprintf(":%d", port)

	// 创建压缩处理器，启用GZIP压缩
	compressedHandler := fasthttp.CompressHandler(handler)

	// 创建fasthttp服务器
	server := &fasthttp.Server{
		Handler:            compressedHandler, // 使用压缩处理器
		Name:               "zhifu-server",
		ReadTimeout:        10 * time.Second,  // 减少读取超时，更快释放资源
		WriteTimeout:       10 * time.Second,  // 减少写入超时，更快释放资源
		IdleTimeout:        120 * time.Second, // 增加空闲连接超时，提高连接复用率
		MaxRequestBodySize: 10 * 1024 * 1024,  // 10MB
		MaxConnsPerIP:      200,               // 增加每个IP最大连接数
		MaxRequestsPerConn: 2000,              // 增加每个连接最大请求数，提高连接复用率
		Concurrency:        20000,             // 增加最大并发连接数
		DisableKeepalive:   false,             // 启用长连接
		ReduceMemoryUsage:  true,              // 启用内存使用优化
		// 启用HTTP/2支持
		NoDefaultServerHeader: true,           // 禁用默认服务器头部，提高安全性
		NoDefaultDate:         true,           // 禁用默认日期头部，减少响应大小
		NoDefaultContentType:  false,          // 保持默认内容类型
	}

	// 检查并清理端口占用
	log.Printf("Checking port %d availability...", port)
	if err := utils.KillProcessUsingPort(port); err != nil {
		log.Printf("Warning: Failed to kill process using port %d: %v", port, err)
	}
	// 短暂延迟确保端口释放
	time.Sleep(1 * time.Second)

	// 创建TCP监听器
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	log.Printf("Server running on http://localhost%s", addr)
	log.Printf("Using fasthttp for improved performance")

	if err := server.Serve(listener); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
