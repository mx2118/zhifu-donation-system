package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/zhifu/donation-rank/models"
	"github.com/zhifu/donation-rank/services"
	"github.com/zhifu/donation-rank/utils"
)

type APIRoutes struct {
	paymentService *services.PaymentService
	baseDir        string
	// WebSocket相关
	upgrader   websocket.Upgrader
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.Mutex
}

func NewAPIRoutes(paymentService *services.PaymentService) *APIRoutes {
	ar := &APIRoutes{
		paymentService: paymentService,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源的WebSocket连接
			},
		},
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}

	// 启动WebSocket处理协程
	go ar.runWebSocketServer()

	return ar
}

// SetupRoutes 设置路由
func (ar *APIRoutes) SetupRoutes(router *gin.Engine, baseDir string) {
	ar.baseDir = baseDir

	api := router.Group("/api")
	{
		api.POST("/donate", ar.CreateDonation) // JSON API，用于AJAX请求
		api.POST("/callback", ar.HandleCallback)
		api.GET("/rankings", ar.GetRankings)
		api.POST("/activate", ar.ActivateTerminal)          // 手动终端激活API
		api.GET("/check-user", ar.CheckUserExists)          // 检查用户是否存在
		api.GET("/payment-config/:id", ar.GetPaymentConfig) // 获取支付配置信息
		api.GET("/category/:id", ar.GetCategory)            // 获取类目信息
		api.GET("/categories", ar.GetCategories)            // 获取所有类目列表
		api.POST("/test-broadcast", ar.TestBroadcast)       // 测试WebSocket广播
		api.POST("/trigger-callback", ar.TriggerCallback)   // 触发支付回调广播测试
	}

	// WebSocket路由
	router.GET("/ws", ar.WebSocketHandler)

	// 微信公众号授权相关路由
	router.GET("/api/wechat/auth", ar.WechatAuth)             // 微信授权入口
	router.GET("/api/wechat/callback", ar.WechatAuthCallback) // 微信授权回调

	// 支付宝授权相关路由
	router.GET("/api/alipay/auth", ar.AlipayAuth)             // 支付宝授权入口
	router.GET("/api/alipay/callback", ar.AlipayAuthCallback) // 支付宝授权回调

	// 表单提交支付（用于302重定向）
	router.POST("/api/donate/form", ar.CreateDonationForm)
	router.GET("/api/donate/form", ar.CreateDonationForm)

	// 生成统一支付二维码
	router.GET("/qrcode", ar.GenerateQRCode)

	// 静态文件服务
	router.Static("/static", filepath.Join(baseDir, "static"))

	// 首页
	router.GET("/", func(c *gin.Context) {
		// 统一使用index.html模板，根据payment参数动态加载内容
		c.File(filepath.Join(baseDir, "templates/index.html"))
	})

	// 支付页面 - 支持动态参数
	router.GET("/pay", func(c *gin.Context) {
		// 始终使用统合的pay.html模板，不管是否有payment参数
		c.File(filepath.Join(baseDir, "templates/pay.html"))
	})

	// 静态文件缓存中间件
	router.Use(func(c *gin.Context) {
		// 为静态文件添加缓存头
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Header("Cache-Control", "public, max-age=86400") // 24小时缓存
			c.Header("Expires", time.Now().Add(24*time.Hour).Format(time.RFC1123))
		}
		c.Next()
	})
}

// CreateDonation 创建捐款订单（JSON API）
func (ar *APIRoutes) CreateDonation(c *gin.Context) {
	// 创建带超时的上下文，设置15秒超时
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// 使用超时上下文替换原请求上下文
	c.Request = c.Request.WithContext(ctx)

	var req struct {
		Amount   float64 `json:"amount" binding:"required"`
		Payment  string  `json:"payment" binding:"required,oneof=wechat alipay"`
		Category string  `json:"category"` // 捐款类目
		Blessing string  `json:"blessing"` // 祝福语
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 手动验证金额范围（使用浮点数比较，配合epsilon处理精度问题）
	epsilon := 0.0001 // 0.01分的精度误差
	if req.Amount < 0.01-epsilon || req.Amount > 10000+epsilon {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be between 0.01 and 10000"})
		return
	}

	// 获取请求的主机名
	host := c.Request.Host

	// 从cookie中获取对应的用户标识
	var openid string
	if req.Payment == "wechat" {
		// 微信用户，从cookie中获取openid
		openid, _ = c.Cookie("wechat_openid")
	} else {
		// 支付宝用户，从cookie中获取user_id
		openid, _ = c.Cookie("alipay_user_id")
	}

	// 确保未授权时openid为"anonymous"
	if openid == "" {
		openid = "anonymous"
	}
	// 获取payment_configs的ID（从请求参数中获取）
	paymentConfigID := c.Query("payment")

	// 使用goroutine和channel处理超时
	type result struct {
		orderID string
		payURL  string
		err     error
	}

	resultChan := make(chan result, 1)

	go func() {
		orderID, payURL, err := ar.paymentService.CreateOrder(req.Amount, req.Payment, host, openid, req.Category, paymentConfigID, req.Blessing)
		resultChan <- result{orderID, payURL, err}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"order_id": res.orderID,
			"pay_url":  res.payURL,
		})
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "请求超时，请稍后再试"})
		return
	}
}

// CreateDonationForm 创建捐款订单（表单提交，302重定向）
func (ar *APIRoutes) CreateDonationForm(c *gin.Context) {
	// 创建带超时的上下文，设置15秒超时
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// 使用超时上下文替换原请求上下文
	c.Request = c.Request.WithContext(ctx)

	// 从表单获取参数
	amountStr := c.PostForm("amount")
	payment := c.PostForm("payment")
	category := c.PostForm("category") // 捐款类目
	blessing := c.PostForm("blessing") // 祝福语

	// 验证参数
	if amountStr == "" || payment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameters"})
		return
	}

	// 转换金额
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
		return
	}

	// 验证支付方式
	if payment != "wechat" && payment != "alipay" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment type"})
		return
	}

	// 获取请求的主机名
	host := c.Request.Host

	// 从cookie中获取对应的用户标识
	var openid string
	if payment == "wechat" {
		// 微信用户，从cookie中获取openid
		openid, _ = c.Cookie("wechat_openid")
	} else {
		// 支付宝用户，从cookie中获取user_id
		openid, _ = c.Cookie("alipay_user_id")
	}

	// 确保未授权时openid为"anonymous"
	if openid == "" {
		openid = "anonymous"
	}
	// 获取payment_configs的ID（从表单或URL参数中获取）
	paymentConfigID := c.PostForm("payment_config_id")
	if paymentConfigID == "" {
		paymentConfigID = c.Query("payment")
	}

	// 使用goroutine和channel处理超时
	type result struct {
		payURL string
		err    error
	}

	resultChan := make(chan result, 1)

	go func() {
		_, payURL, err := ar.paymentService.CreateOrder(amount, payment, host, openid, category, paymentConfigID, blessing)
		resultChan <- result{payURL, err}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.err.Error()})
			return
		}

		// 302重定向到支付URL（根据API文档Step 3要求）
		c.Redirect(http.StatusFound, res.payURL)
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "请求超时，请稍后再试"})
		return
	}
}

// CheckUserExists 检查用户是否存在
func (ar *APIRoutes) CheckUserExists(c *gin.Context) {
	openid := c.Query("openid")
	payment := c.Query("payment")

	if openid == "" || payment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameters"})
		return
	}

	exists := false

	if payment == "wechat" {
		// 检查微信用户是否存在
		var wechatUser models.WechatUser
		if err := utils.DB.Where("open_id = ?", openid).First(&wechatUser).Error; err == nil {
			exists = true
		}
	} else if payment == "alipay" {
		// 检查支付宝用户是否存在
		var alipayUser models.AlipayUser
		if err := utils.DB.Where("user_id = ?", openid).First(&alipayUser).Error; err == nil {
			exists = true
		}
	}

	c.JSON(http.StatusOK, gin.H{"exists": exists})
}

// WechatAuth 微信公众号授权入口
func (ar *APIRoutes) WechatAuth(c *gin.Context) {
	// 获取当前主机名
	host := c.Request.Host

	// 获取重定向URL参数
	redirectURL := c.Query("redirect_url")

	// 获取payment和categories参数
	payment := c.Query("payment")
	categories := c.Query("categories")

	if redirectURL == "" {
		// 默认重定向到支付页面
		redirectURL = fmt.Sprintf("http://%s/pay", host)

		// 添加参数
		firstParam := true
		if payment != "" {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
			firstParam = false
			if categories != "" {
				redirectURL += fmt.Sprintf("&categories=%s", categories)
			}
		} else if categories != "" {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
			firstParam = false
		}

		// 添加authorized参数
		if firstParam {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	} else {
		// 如果重定向URL不包含authorized参数，添加它
		if !strings.Contains(redirectURL, "?") {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	}

	// 如果重定向URL中没有payment和categories参数，但请求中有，添加它们
	if payment != "" && !strings.Contains(redirectURL, "payment=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
		} else {
			redirectURL += fmt.Sprintf("&payment=%s", payment)
		}
	}

	if categories != "" && !strings.Contains(redirectURL, "categories=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
		} else {
			redirectURL += fmt.Sprintf("&categories=%s", categories)
		}
	}

	// 生成授权URL并跳转
	authURL, err := ar.paymentService.GetWechatAuthURLWithRedirect(host, redirectURL)
	if err != nil {
		log.Printf("Failed to generate wechat auth URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate auth URL"})
		return
	}

	// 302重定向到微信授权页面
	c.Redirect(http.StatusFound, authURL)
}

// WechatAuthCallback 微信公众号授权回调处理
func (ar *APIRoutes) WechatAuthCallback(c *gin.Context) {
	// 获取授权码
	code := c.Query("code")

	// 获取重定向URL参数
	redirectURL := c.Query("redirect_url")

	// 获取payment和categories参数
	payment := c.Query("payment")
	categories := c.Query("categories")

	if code == "" {
		// 未获取到授权码，设置为匿名施主
		c.SetCookie("wechat_openid", "anonymous", 86400, "/", "", false, false)
		c.SetCookie("wechat_user_id", "anonymous", 86400, "/", "", false, false)
		c.SetCookie("wechat_user_name", "匿名施主", 86400, "/", "", false, false)
		// 设置默认头像URL
		c.SetCookie("wechat_avatar_url", "./static/avatar.jpeg", 86400, "/", "", false, false)

		// 构建重定向URL
		if redirectURL == "" {
			// 默认重定向到支付页面
			redirectURL = "/pay"

			// 添加payment和categories参数
			firstParam := true
			if payment != "" {
				redirectURL += fmt.Sprintf("?payment=%s", payment)
				firstParam = false
				if categories != "" {
					redirectURL += fmt.Sprintf("&categories=%s", categories)
				}
			} else if categories != "" {
				redirectURL += fmt.Sprintf("?categories=%s", categories)
				firstParam = false
			}

			// 添加authorized参数
			if firstParam {
				redirectURL += "?authorized=1"
			} else {
				redirectURL += "&authorized=1"
			}
		} else {
			// 如果重定向URL不包含authorized参数，添加它
			if !strings.Contains(redirectURL, "?") {
				redirectURL += "?authorized=1"
			} else {
				redirectURL += "&authorized=1"
			}
		}

		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	// 构建重定向URL
	if redirectURL == "" {
		// 默认重定向到支付页面
		redirectURL = "/pay"

		// 添加payment和categories参数
		firstParam := true
		if payment != "" {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
			firstParam = false
			if categories != "" {
				redirectURL += fmt.Sprintf("&categories=%s", categories)
			}
		} else if categories != "" {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
			firstParam = false
		}

		// 添加authorized参数
		if firstParam {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	} else {
		// 如果重定向URL不包含authorized参数，添加它
		if !strings.Contains(redirectURL, "?") {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}

		// 如果重定向URL中没有payment和categories参数，但请求中有，添加它们
		if payment != "" && !strings.Contains(redirectURL, "payment=") {
			redirectURL += fmt.Sprintf("&payment=%s", payment)
		}

		if categories != "" && !strings.Contains(redirectURL, "categories=") {
			redirectURL += fmt.Sprintf("&categories=%s", categories)
		}
	}

	// 如果重定向URL中没有payment和categories参数，但请求中有，添加它们
	if payment != "" && !strings.Contains(redirectURL, "payment=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
		} else {
			redirectURL += fmt.Sprintf("&payment=%s", payment)
		}
	}

	if categories != "" && !strings.Contains(redirectURL, "categories=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
		} else {
			redirectURL += fmt.Sprintf("&categories=%s", categories)
		}
	}

	// 使用授权码获取用户信息
	userInfo, err := ar.paymentService.GetWechatUserInfoByCode(code)
	if err != nil {
		log.Printf("Failed to get wechat user info by code: %v", err)
		// 授权失败，设置为匿名施主
		c.SetCookie("wechat_openid", "anonymous", 86400, "/", "", false, true)
		c.SetCookie("wechat_user_id", "anonymous", 86400, "/", "", false, true)
		c.SetCookie("wechat_user_name", "匿名施主", 86400, "/", "", false, true)
		// 设置默认头像URL
		c.SetCookie("wechat_avatar_url", "./static/avatar.jpeg", 86400, "/", "", false, true)
		// 重定向回原页面
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	// 将用户信息存储到cookie中，方便后续使用
	c.SetCookie("wechat_openid", userInfo["openid"].(string), 86400, "/", "", false, false)
	c.SetCookie("wechat_user_id", userInfo["openid"].(string), 86400, "/", "", false, false)
	if nickname, ok := userInfo["nickname"].(string); ok {
		c.SetCookie("wechat_user_name", url.QueryEscape(nickname), 86400, "/", "", false, false)
	}
	if headimgurl, ok := userInfo["headimgurl"].(string); ok {
		c.SetCookie("wechat_avatar_url", url.QueryEscape(headimgurl), 86400, "/", "", false, false)
	}

	// 重定向回原页面，添加授权标记
	c.Redirect(http.StatusFound, redirectURL)
}

// AlipayAuth 支付宝授权入口
func (ar *APIRoutes) AlipayAuth(c *gin.Context) {
	// 获取当前主机名
	host := c.Request.Host

	// 获取重定向URL参数
	redirectURL := c.Query("redirect_url")

	// 获取payment和categories参数
	payment := c.Query("payment")
	categories := c.Query("categories")

	if redirectURL == "" {
		// 默认重定向到支付页面
		redirectURL = fmt.Sprintf("http://%s/pay", host)

		// 添加参数
		firstParam := true
		if payment != "" {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
			firstParam = false
			if categories != "" {
				redirectURL += fmt.Sprintf("&categories=%s", categories)
			}
		} else if categories != "" {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
			firstParam = false
		}

		// 添加authorized参数
		if firstParam {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	} else {
		// 如果重定向URL不包含authorized参数，添加它
		if !strings.Contains(redirectURL, "?") {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	}

	// 如果重定向URL中没有payment和categories参数，但请求中有，添加它们
	if payment != "" && !strings.Contains(redirectURL, "payment=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
		} else {
			redirectURL += fmt.Sprintf("&payment=%s", payment)
		}
	}

	if categories != "" && !strings.Contains(redirectURL, "categories=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
		} else {
			redirectURL += fmt.Sprintf("&categories=%s", categories)
		}
	}

	// 生成授权URL并跳转
	authURL, err := ar.paymentService.GetAlipayAuthURLWithRedirect(host, redirectURL)
	if err != nil {
		log.Printf("Failed to generate alipay auth URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate auth URL"})
		return
	}

	// 302重定向到支付宝授权页面
	c.Redirect(http.StatusFound, authURL)
}

// AlipayAuthCallback 支付宝授权回调处理
func (ar *APIRoutes) AlipayAuthCallback(c *gin.Context) {
	// 获取授权码
	code := c.Query("auth_code")

	// 从state参数中获取重定向URL
	redirectURL := c.Query("state")
	// 解码state参数
	var err error
	if redirectURL != "" {
		redirectURL, err = url.QueryUnescape(redirectURL)
		if err != nil {
			log.Printf("Failed to unescape redirect URL: %v", err)
			redirectURL = ""
		}
	}

	// 获取payment和categories参数
	payment := c.Query("payment")
	categories := c.Query("categories")

	// 尝试从redirect_url中解析payment和categories参数
	if payment == "" || categories == "" {
		if redirectURL != "" {
			parsedURL, err := url.Parse(redirectURL)
			if err == nil {
				params := parsedURL.Query()
				if payment == "" {
					payment = params.Get("payment")
				}
				if categories == "" {
					categories = params.Get("categories")
				}
			}
		}
	}

	if code == "" {
		// 未获取到授权码，设置为匿名施主
		c.SetCookie("alipay_user_id", "anonymous", 86400, "/", "", false, false)
		c.SetCookie("alipay_user_name", "匿名施主", 86400, "/", "", false, false)
		// 设置默认头像URL
		c.SetCookie("alipay_avatar_url", "./static/avatar.jpeg", 86400, "/", "", false, false)

		// 构建重定向URL
		if redirectURL == "" {
			// 默认重定向到支付页面
			redirectURL = "/pay"

			// 添加payment和categories参数
			firstParam := true
			if payment != "" {
				redirectURL += fmt.Sprintf("?payment=%s", payment)
				firstParam = false
				if categories != "" {
					redirectURL += fmt.Sprintf("&categories=%s", categories)
				}
			} else if categories != "" {
				redirectURL += fmt.Sprintf("?categories=%s", categories)
				firstParam = false
			}

			// 添加authorized参数
			if firstParam {
				redirectURL += "?authorized=1"
			} else {
				redirectURL += "&authorized=1"
			}
		} else {
			// 如果重定向URL不包含authorized参数，添加它
			if !strings.Contains(redirectURL, "?") {
				redirectURL += "?authorized=1"
			} else {
				redirectURL += "&authorized=1"
			}
		}

		// 重定向回原页面
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	// 构建重定向URL
	if redirectURL == "" {
		// 默认重定向到支付页面
		redirectURL = "/pay"

		// 添加payment和categories参数
		firstParam := true
		if payment != "" {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
			firstParam = false
			if categories != "" {
				redirectURL += fmt.Sprintf("&categories=%s", categories)
			}
		} else if categories != "" {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
			firstParam = false
		}

		// 添加authorized参数
		if firstParam {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	} else {
		// 如果重定向URL不包含authorized参数，添加它
		if !strings.Contains(redirectURL, "?") {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}

		// 如果重定向URL中没有payment和categories参数，但请求中有，添加它们
		if payment != "" && !strings.Contains(redirectURL, "payment=") {
			redirectURL += fmt.Sprintf("&payment=%s", payment)
		}

		if categories != "" && !strings.Contains(redirectURL, "categories=") {
			redirectURL += fmt.Sprintf("&categories=%s", categories)
		}
	}

	// 如果重定向URL中没有payment和categories参数，但请求中有，添加它们
	if payment != "" && !strings.Contains(redirectURL, "payment=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
		} else {
			redirectURL += fmt.Sprintf("&payment=%s", payment)
		}
	}

	if categories != "" && !strings.Contains(redirectURL, "categories=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
		} else {
			redirectURL += fmt.Sprintf("&categories=%s", categories)
		}
	}

	// 使用授权码获取用户信息
	userInfo, err := ar.paymentService.GetAlipayUserInfoByCode(code)
	if err != nil {
		log.Printf("Failed to get alipay user info by code: %v", err)
		// 授权失败，设置为匿名施主
		c.SetCookie("alipay_user_id", "anonymous", 86400, "/", "", false, true)
		c.SetCookie("alipay_user_name", "匿名施主", 86400, "/", "", false, true)
		// 设置默认头像URL
		c.SetCookie("alipay_avatar_url", "./static/avatar.jpeg", 86400, "/", "", false, true)
		// 重定向回原页面
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	// 将用户信息存储到cookie中，方便后续使用
	userID := userInfo["user_id"]
	userName := userInfo["user_name"]
	avatarURL := userInfo["avatar_url"]
	accessToken := userInfo["access_token"]

	// 对包含特殊字符的值进行URL编码，确保cookie存储正确
	c.SetCookie("alipay_user_id", userID, 86400, "/", "", false, false)
	c.SetCookie("alipay_user_name", url.QueryEscape(userName), 86400, "/", "", false, false)
	c.SetCookie("alipay_avatar_url", url.QueryEscape(avatarURL), 86400, "/", "", false, false)

	// 保存access_token到cookie中，用于后续获取用户信息
	if accessToken != "" {
		c.SetCookie("alipay_access_token", accessToken, 86400, "/", "", false, false)
		// 保存access_token到数据库用户表中
		var alipayUser models.AlipayUser
		if err := utils.DB.Where("user_id = ?", userID).FirstOrCreate(&alipayUser, models.AlipayUser{UserID: userID}).Error; err == nil {
			alipayUser.AccessToken = accessToken
			alipayUser.Nickname = userName
			alipayUser.AvatarURL = avatarURL
			utils.DB.Save(&alipayUser)
		}
	}

	// 重定向回原页面，添加授权标记
	c.Redirect(http.StatusFound, redirectURL)
}

// HandleCallback 处理支付回调（WAP支付方式）
func (ar *APIRoutes) HandleCallback(c *gin.Context) {
	log.Printf("====================================")
	log.Printf("开始处理支付回调")
	log.Printf("当前时间: %v", time.Now())
	log.Printf("====================================")

	// 读取请求体
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("Failed to read callback body: %v", err)
		c.String(http.StatusBadRequest, "error reading body")
		return
	}

	// 记录完整的回调请求日志
	log.Printf("Received callback request: Method=%s, URL=%s, Headers=%v, Body=%s",
		c.Request.Method, c.Request.URL.String(), c.Request.Header, string(body))

	// 解析JSON数据，使用map[string]interface{}处理数组字段
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("Failed to parse callback JSON: %v, Body: %s", err, string(body))
		c.String(http.StatusBadRequest, "invalid json")
		return
	}

	// 获取订单号
	orderID, _ := data["client_sn"].(string)
	if orderID == "" {
		log.Printf("Missing client_sn in callback: %v", data)
		c.String(http.StatusBadRequest, "missing client_sn")
		return
	}

	// 获取Authorization头中的sign
	auth := c.GetHeader("Authorization")
	log.Printf("Callback for order %s, auth header: %s", orderID, auth)

	// 处理回调，支持两种签名验证方式
	var handleErr error
	if auth != "" {
		// 方式1：使用RSA公钥验证（推荐）
		log.Printf("Using RSA public key to verify callback for order %s", orderID)
		handleErr = ar.paymentService.HandleCallbackWithPublicKey(data, auth, body)
	} else if sign, ok := data["sign"].(string); ok && sign != "" {
		// 方式2：使用终端密钥验证（兼容旧版）
		log.Printf("Using terminal key to verify callback for order %s", orderID)
		handleErr = ar.paymentService.HandleCallback(data)
	} else {
		log.Printf("No sign found in callback for order %s", orderID)
		c.String(http.StatusBadRequest, "missing sign")
		return
	}

	// 处理回调结果
	if handleErr != nil {
		log.Printf("Callback handle error for order %s: %v", orderID, handleErr)
		c.String(http.StatusInternalServerError, "error handling callback")
		return
	}

	// 同步获取与当前订单相关的捐款记录并广播，确保广播成功
	log.Printf("开始同步获取与当前订单相关的捐款记录并广播，订单ID: %s", orderID)
	// 短暂延迟，确保数据库事务已提交
	time.Sleep(1 * time.Second)

	// 获取与当前订单相关的捐款记录
	donation, err := ar.paymentService.GetDonationByOrderID(orderID)
	if err != nil {
		log.Printf("获取与订单相关的捐款记录失败: %v", err)
	} else if donation != nil {
		// 检查支付状态是否为completed
		if donation.Status == "completed" {
			log.Printf("获取到已完成的捐款记录: ID=%d, Amount=%.2f, Payment=%s, PaymentConfigID=%s, Categories=%s, Status=%s",
				donation.ID, donation.Amount, donation.Payment,
				donation.PaymentConfigID, donation.Categories, donation.Status)
			// 广播新的捐款记录
			ar.BroadcastNewDonation(donation)
		} else {
			log.Printf("捐款记录状态不是completed，跳过广播: Status=%s", donation.Status)
		}
	} else {
		log.Printf("未获取到与订单相关的捐款记录")
	}

	log.Printf("Callback handled successfully for order %s", orderID)
	// 返回success
	c.String(http.StatusOK, "success")
}

// GetRankings 获取捐款排行榜
func (ar *APIRoutes) GetRankings(c *gin.Context) {
	// 创建带超时的上下文，设置10秒超时
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// 使用超时上下文替换原请求上下文
	c.Request = c.Request.WithContext(ctx)

	// 解析limit参数，设置默认值和范围校验
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}
	// 限制最大返回数量，防止性能问题
	if limit > 100 {
		limit = 100
	}

	// 解析page参数，设置默认值和范围校验
	pageStr := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}

	// 计算偏移量
	// 获取payment和categories参数
	paymentConfigID := c.Query("payment")
	categoryID := c.Query("categories")

	// 计算偏移量
	offset := (page - 1) * limit

	// 使用goroutine和channel处理超时
	type result struct {
		rankings []services.RankingItem
		err      error
	}

	resultChan := make(chan result, 1)

	go func() {
		rankings, err := ar.paymentService.GetRankings(limit, offset, paymentConfigID, categoryID)
		resultChan <- result{rankings, err}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": res.err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"rankings": res.rankings,
			"pagination": gin.H{
				"limit":  limit,
				"page":   page,
				"offset": offset,
				"total":  len(res.rankings),
			},
		})
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "请求超时，请稍后再试"})
		return
	}
}

// ActivateTerminal 手动激活终端API
func (ar *APIRoutes) ActivateTerminal(c *gin.Context) {
	// 从请求体获取激活码
	var req struct {
		ActivationCode string `json:"activation_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 执行终端激活
	if err := ar.paymentService.ActivateTerminal(req.ActivationCode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Terminal activation failed: %v", err),
		})
		return
	}

	// 获取激活后的终端配置
	config := ar.paymentService.Config()

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message":      "Terminal activation successful",
		"terminal_sn":  config.TerminalSN,
		"terminal_key": config.TerminalKey,
	})
}

// GenerateQRCode 生成统一支付二维码
func (ar *APIRoutes) GenerateQRCode(c *gin.Context) {
	// 获取payment参数
	payment := c.Query("payment")

	// 如果payment参数不存在，返回首页
	if payment == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	// 获取categories参数
	categories := c.Query("categories")

	// 当payment有参数时，如果没有categories参数，自动设置默认的categories参数
	if categories == "" {
		// 设置默认的categories参数为 "1"
		categories = "1"
	}

	// 获取请求的主机名
	host := c.Request.Host

	// 处理不同的访问情况
	switch host {
	// 本地访问情况
	case "localhost:8080", "localhost:9090", ":8080", ":9090":
		// 使用第一个局域网IP地址（仅用于本地测试）
		host = "192.168.19.52:9090"
	// 远程服务器访问情况
	default:
		// 直接使用请求的host，确保远程访问时使用正确的域名/IP
		// 例如：101.34.24.139:9090
	}

	// 生成支付页面URL
	payURL := fmt.Sprintf("http://%s/pay", host)

	// 添加参数
	payURL += fmt.Sprintf("?payment=%s", payment)
	if categories != "" {
		payURL += fmt.Sprintf("&categories=%s", categories)
	}

	qrBytes, err := utils.GenerateQRCode(payURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "image/png")
	c.Writer.Write(qrBytes)
}

// GetPaymentConfig 获取支付配置信息
func (ar *APIRoutes) GetPaymentConfig(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少支付配置ID参数"})
		return
	}

	var paymentConfig models.PaymentConfig
	if err := utils.DB.Where("id = ?", id).First(&paymentConfig).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "支付配置不存在"})
		return
	}

	c.JSON(http.StatusOK, paymentConfig)
}

// GetCategory 获取类目信息
func (ar *APIRoutes) GetCategory(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少类目ID参数"})
		return
	}

	var category models.Category
	if err := utils.DB.Where("id = ?", id).First(&category).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "类目不存在"})
		return
	}

	c.JSON(http.StatusOK, category)
}

// GetCategories 获取所有类目列表
func (ar *APIRoutes) GetCategories(c *gin.Context) {
	var categories []models.Category
	query := utils.DB

	// 根据payment参数过滤
	payment := c.Query("payment")
	if payment != "" {
		query = query.Where("payment = ?", payment)
	}

	if err := query.Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取类目列表失败"})
		return
	}

	c.JSON(http.StatusOK, categories)
}

// runWebSocketServer 运行WebSocket服务器
func (ar *APIRoutes) runWebSocketServer() {
	log.Printf("====================================")
	log.Printf("WebSocket服务器已启动")
	log.Printf("当前时间: %v", time.Now())
	log.Printf("====================================")

	// 定期清理无效连接的定时器
	cleanupTicker := time.NewTicker(30 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case client := <-ar.register:
			ar.mutex.Lock()
			ar.clients[client] = true
			clientCount := len(ar.clients)
			ar.mutex.Unlock()
			log.Printf("====================================")
			log.Printf("WebSocket客户端已连接")
			log.Printf("当前客户端数量: %d", clientCount)
			log.Printf("====================================")

			// 发送初始数据
			go ar.sendInitialData(client)

		case client := <-ar.unregister:
			ar.mutex.Lock()
			if _, ok := ar.clients[client]; ok {
				delete(ar.clients, client)
				client.Close()
			}
			clientCount := len(ar.clients)
			ar.mutex.Unlock()
			log.Printf("====================================")
			log.Printf("WebSocket客户端已断开连接")
			log.Printf("当前客户端数量: %d", clientCount)
			log.Printf("====================================")

		case message := <-ar.broadcast:
			ar.mutex.Lock()
			clientCount := len(ar.clients)
			ar.mutex.Unlock()

			log.Printf("====================================")
			log.Printf("开始处理广播消息")
			log.Printf("当前客户端数量: %d", clientCount)
			log.Printf("消息大小: %d bytes", len(message))
			log.Printf("====================================")

			if clientCount == 0 {
				log.Printf("没有客户端连接，跳过广播")
				continue
			}

			ar.mutex.Lock()
			successCount := 0
			failCount := 0
			for client := range ar.clients {
				select {
				case <-time.After(1000 * time.Millisecond):
					// 超时，跳过该客户端
					failCount++
					log.Printf("向客户端广播消息超时")
				default:
					if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
						log.Printf("向客户端广播消息失败: %v", err)
						client.Close()
						delete(ar.clients, client)
						failCount++
					} else {
						successCount++
						log.Printf("向客户端广播消息成功")
					}
				}
			}
			ar.mutex.Unlock()
			log.Printf("====================================")
			log.Printf("广播完成")
			log.Printf("成功: %d, 失败: %d, 总客户端数: %d", successCount, failCount, clientCount)
			log.Printf("====================================")

		case <-cleanupTicker.C:
			// 定期清理无效连接
			log.Printf("====================================")
			log.Printf("开始清理无效连接")
			ar.cleanupInvalidConnections()
			ar.mutex.Lock()
			clientCount := len(ar.clients)
			ar.mutex.Unlock()
			log.Printf("清理完成，当前客户端数量: %d", clientCount)
			log.Printf("====================================")
		}
	}
}

// cleanupInvalidConnections 清理无效的WebSocket连接
func (ar *APIRoutes) cleanupInvalidConnections() {
	ar.mutex.Lock()
	defer ar.mutex.Unlock()

	totalClients := len(ar.clients)
	invalidCount := 0

	for client := range ar.clients {
		// 发送ping消息测试连接是否有效
		if err := client.WriteMessage(websocket.PingMessage, nil); err != nil {
			// 连接无效，关闭并从映射中删除
			client.Close()
			delete(ar.clients, client)
			invalidCount++
		}
	}

	if invalidCount > 0 {
		log.Printf("Cleaned up %d invalid WebSocket connections. Total clients: %d → %d",
			invalidCount, totalClients, len(ar.clients))
	}
}

// sendInitialData 发送初始数据给新连接的客户端
func (ar *APIRoutes) sendInitialData(client *websocket.Conn) {
	// 获取最新的功德记录
	rankings, err := ar.paymentService.GetRankings(50, 0, "", "")
	if err != nil {
		log.Printf("Error getting initial rankings: %v", err)
		return
	}

	// 构建初始数据消息
	initialData := map[string]interface{}{
		"type":      "initial_data",
		"rankings":  rankings,
		"timestamp": time.Now().Unix(),
	}

	message, err := json.Marshal(initialData)
	if err != nil {
		log.Printf("Error marshaling initial data: %v", err)
		return
	}

	if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
		log.Printf("Error sending initial data: %v", err)
		client.Close()
		ar.mutex.Lock()
		delete(ar.clients, client)
		ar.mutex.Unlock()
	}
}

// WebSocketHandler 处理WebSocket连接
func (ar *APIRoutes) WebSocketHandler(c *gin.Context) {
	// 升级HTTP连接为WebSocket连接
	conn, err := ar.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}

	// 注册新客户端
	ar.register <- conn

	// 处理客户端消息
	for {
		messageType, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// 忽略客户端发送的消息，只处理服务器推送
		if messageType == websocket.PingMessage {
			if err := conn.WriteMessage(websocket.PongMessage, nil); err != nil {
				break
			}
		}
	}

	// 注销客户端
	ar.unregister <- conn
}

// BroadcastNewDonation 广播新的捐款记录
func (ar *APIRoutes) BroadcastNewDonation(donation interface{}) {
	// 构建广播消息
	message := map[string]interface{}{
		"type":      "new_donation",
		"donation":  donation,
		"timestamp": time.Now().Unix(),
	}

	log.Printf("开始广播新捐款记录，当前客户端数量: %d", len(ar.clients))
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling donation data: %v", err)
		return
	}

	// 发送到广播通道
	ar.broadcast <- data
	log.Printf("广播消息已发送到通道，消息大小: %d bytes", len(data))
}

// TestBroadcast 测试WebSocket广播功能
func (ar *APIRoutes) TestBroadcast(c *gin.Context) {
	log.Printf("====================================")
	log.Printf("收到测试广播请求")
	log.Printf("当前时间: %v", time.Now())
	log.Printf("====================================")

	// 生成测试捐款记录
	testDonation := map[string]interface{}{
		"id":              time.Now().Unix(),
		"user_name":       "测试用户",
		"amount":          100.00,
		"blessing":        "这是一条测试捐款记录，用于测试WebSocket广播功能",
		"avatar_url":      "/static/avatar.jpeg",
		"payment":         "wechat",
		"created_at":      time.Now().Format("2006-01-02 15:04:05"),
		"PaymentConfigID": 2,
		"Categories":      "17",
	}

	log.Printf("生成测试捐款记录: %v", testDonation)

	// 广播测试捐款记录
	ar.BroadcastNewDonation(testDonation)

	log.Printf("测试广播完成")

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "测试广播已发送",
		"donation": testDonation,
	})
}

// TriggerCallback 触发支付回调广播测试
func (ar *APIRoutes) TriggerCallback(c *gin.Context) {
	log.Printf("====================================")
	log.Printf("收到触发回调广播请求")
	log.Printf("当前时间: %v", time.Now())
	log.Printf("====================================")

	// 模拟支付回调的捐款记录
	testDonation := map[string]interface{}{
		"id":              time.Now().Unix(),
		"user_name":       "实际用户",
		"amount":          50.00,
		"blessing":        "支付回调测试捐款",
		"avatar_url":      "/static/avatar.jpeg",
		"payment":         "wechat",
		"created_at":      time.Now().Format("2006-01-02 15:04:05"),
		"PaymentConfigID": 2,
		"Categories":      "17",
		"order_id":        fmt.Sprintf("ORD%d", time.Now().Unix()),
		"status":          "completed",
	}

	log.Printf("模拟支付回调捐款记录: %v", testDonation)

	// 广播捐款记录
	ar.BroadcastNewDonation(testDonation)

	log.Printf("回调广播完成")

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "支付回调广播已触发",
		"donation": testDonation,
	})
}
