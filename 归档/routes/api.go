package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/zhifu/donation-rank/models"
	"github.com/zhifu/donation-rank/services"
	"github.com/zhifu/donation-rank/utils"
)

type APIRoutes struct {
	paymentService *services.PaymentService
	wsManager      *WebSocketManager
	baseDir        string
}

func NewAPIRoutes(paymentService *services.PaymentService) *APIRoutes {
	wsManager := NewWebSocketManager()
	return &APIRoutes{
		paymentService: paymentService,
		wsManager:      wsManager,
	}
}

// HandleRequest 处理fasthttp请求
func (ar *APIRoutes) HandleRequest(ctx *fasthttp.RequestCtx, baseDir string) {
	ar.baseDir = baseDir

	path := string(ctx.Path())
	method := string(ctx.Method())

	// 详细调试信息
	log.Printf("[DEBUG] Full request: path='%s', method='%s', IP='%s'", path, method, string(ctx.RemoteIP().String()))

	// 检查特定路径
	if path == "/api/pay/callback" {
		log.Printf("[DEBUG] Matched /api/pay/callback path!")
	}

	if path == "/api/callback" {
		log.Printf("[DEBUG] Matched /api/callback path!")
	}

	// 处理WebSocket路径
	if path == "/ws/pay-notify" {
		// 获取WebSocket参数（支持别名）
		payment := string(ctx.QueryArgs().Peek("payment"))
		if payment == "" {
			payment = string(ctx.QueryArgs().Peek("p"))
		}
		categories := string(ctx.QueryArgs().Peek("categories"))
		if categories == "" {
			categories = string(ctx.QueryArgs().Peek("c"))
		}
		fmt.Printf("[DEBUG] WebSocket connection attempt: path='%s', method='%s', IP='%s', payment='%s', categories='%s'\n", path, method, string(ctx.RemoteIP().String()), payment, categories)
		ar.wsManager.HandleWebSocket(ctx)
		return
	}

	// 处理静态文件
	if strings.HasPrefix(path, "/static/") {
		ar.serveStaticFile(ctx, path)
		return
	}

	// 处理API路由
	switch {
	// API路由
	case path == "/api/donate" && method == "POST":
		ar.CreateDonation(ctx)
	case (path == "/api/callback" || path == "/api/pay/callback") && method == "POST":
		ar.HandleCallback(ctx)
	case path == "/api/rankings" && method == "GET":
		ar.GetRankings(ctx)
	case path == "/api/activate" && method == "POST":
		ar.ActivateTerminal(ctx)
	case path == "/api/check-user" && method == "GET":
		ar.CheckUserExists(ctx)
	case strings.HasPrefix(path, "/api/payment-config/") && method == "GET":
		ar.GetPaymentConfig(ctx)
	case strings.HasPrefix(path, "/api/category/") && method == "GET":
		ar.GetCategory(ctx)
	case path == "/api/categories" && method == "GET":
		ar.GetCategories(ctx)

	// 微信授权路由
	case path == "/api/wechat/auth" && method == "GET":
		ar.WechatAuth(ctx)
	case path == "/api/wechat/callback" && method == "GET":
		ar.WechatAuthCallback(ctx)

	// 支付宝授权路由
	case path == "/api/alipay/auth" && method == "GET":
		ar.AlipayAuth(ctx)
	case path == "/api/alipay/callback" && method == "GET":
		ar.AlipayAuthCallback(ctx)

	// 表单提交支付
	case path == "/api/donate/form":
		ar.CreateDonationForm(ctx)

	// 生成二维码
	case path == "/qrcode" && method == "GET":
		ar.GenerateQRCode(ctx)

	// 首页，支持带参数访问
	case path == "/" && method == "GET":
		// 获取参数（支持别名）
		payment := string(ctx.QueryArgs().Peek("payment"))
		if payment == "" {
			payment = string(ctx.QueryArgs().Peek("p"))
		}
		categories := string(ctx.QueryArgs().Peek("categories"))
		if categories == "" {
			categories = string(ctx.QueryArgs().Peek("c"))
		}
		log.Printf("Home page accessed with payment=%s, categories=%s", payment, categories)
		// 提供正式的业务逻辑页面
		ar.serveTemplate(ctx, "templates/index.html")

	// 支付页面
	case path == "/pay" && method == "GET":
		ar.serveTemplate(ctx, "templates/pay.html")
	default:
		log.Printf("404 Not Found: path=%s, method=%s", path, method)
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.WriteString("Not Found")
	}
}

// serveStaticFile 提供静态文件服务
func (ar *APIRoutes) serveStaticFile(ctx *fasthttp.RequestCtx, path string) {
	// 为静态文件添加缓存头
	ctx.Response.Header.Set("Cache-Control", "public, max-age=86400") // 24小时缓存
	ctx.Response.Header.Set("Expires", time.Now().Add(24*time.Hour).Format(time.RFC1123))

	// 提供文件
	fasthttp.ServeFile(ctx, filepath.Join(ar.baseDir, path))
}

// serveTemplate 提供模板文件服务
func (ar *APIRoutes) serveTemplate(ctx *fasthttp.RequestCtx, templatePath string) {
	// 提供文件
	fasthttp.ServeFile(ctx, filepath.Join(ar.baseDir, templatePath))
}

// CreateDonation 创建捐款订单（JSON API）
func (ar *APIRoutes) CreateDonation(ctx *fasthttp.RequestCtx) {
	// 创建带超时的上下文，设置15秒超时
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var req struct {
		Amount   float64 `json:"amount"`
		Payment  string  `json:"payment"`
		Category string  `json:"category"` // 捐款类目
		Blessing string  `json:"blessing"` // 祝福语
	}

	// 解析请求体
	if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": err.Error()})
		return
	}

	// 验证参数
	if req.Payment == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "payment is required"})
		return
	}

	// 手动验证金额范围（使用浮点数比较，配合epsilon处理精度问题）
	epsilon := 0.0001 // 0.01分的精度误差
	if req.Amount < 0.01-epsilon || req.Amount > 10000+epsilon {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "amount must be between 0.01 and 10000"})
		return
	}

	// 获取请求的主机名
	host := string(ctx.Host())

	// 从cookie中获取对应的用户标识
	var openid string
	if req.Payment == "wechat" {
		// 微信用户，从cookie中获取openid
		openid = string(ctx.Request.Header.Cookie("wechat_openid"))
	} else {
		// 支付宝用户，从cookie中获取user_id
		openid = string(ctx.Request.Header.Cookie("alipay_user_id"))
	}

	// 确保未授权时openid为"anonymous"
	if openid == "" {
		openid = "anonymous"
	}
	// 获取payment_configs的ID（从请求参数中获取）
	paymentConfigID := string(ctx.QueryArgs().Peek("payment"))

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
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(ctx).Encode(map[string]string{"error": res.err.Error()})
			return
		}

		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{
			"order_id": res.orderID,
			"pay_url":  res.payURL,
		})
	case <-ctxTimeout.Done():
		ctx.SetStatusCode(fasthttp.StatusRequestTimeout)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "请求超时，请稍后再试"})
		return
	}
}

// CreateDonationForm 创建捐款订单（表单提交，302重定向）
func (ar *APIRoutes) CreateDonationForm(ctx *fasthttp.RequestCtx) {
	// 创建带超时的上下文，设置15秒超时
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 从表单获取参数
	amountStr := string(ctx.FormValue("amount"))
	payment := string(ctx.FormValue("payment"))
	category := string(ctx.FormValue("category")) // 捐款类目
	blessing := string(ctx.FormValue("blessing")) // 祝福语

	// 验证参数
	if amountStr == "" || payment == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "missing required parameters"})
		return
	}

	// 转换金额
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "invalid amount"})
		return
	}

	// 验证支付方式
	if payment != "wechat" && payment != "alipay" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "invalid payment type"})
		return
	}

	// 获取请求的主机名
	host := string(ctx.Host())

	// 从cookie中获取对应的用户标识
	var openid string
	if payment == "wechat" {
		// 微信用户，从cookie中获取openid
		openid = string(ctx.Request.Header.Cookie("wechat_openid"))
	} else {
		// 支付宝用户，从cookie中获取user_id
		openid = string(ctx.Request.Header.Cookie("alipay_user_id"))
	}

	// 确保未授权时openid为"anonymous"
	if openid == "" {
		openid = "anonymous"
	}
	// 获取payment_configs的ID（从表单或URL参数中获取，支持别名）
	paymentConfigID := string(ctx.FormValue("payment_config_id"))
	if paymentConfigID == "" {
		paymentConfigID = string(ctx.QueryArgs().Peek("payment"))
		if paymentConfigID == "" {
			paymentConfigID = string(ctx.QueryArgs().Peek("p"))
		}
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
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(ctx).Encode(map[string]string{"error": res.err.Error()})
			return
		}

		// 302重定向到支付URL（根据API文档Step 3要求）
		ctx.Redirect(res.payURL, fasthttp.StatusFound)
	case <-ctxTimeout.Done():
		ctx.SetStatusCode(fasthttp.StatusRequestTimeout)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "请求超时，请稍后再试"})
		return
	}
}

// CheckUserExists 检查用户是否存在
func (ar *APIRoutes) CheckUserExists(ctx *fasthttp.RequestCtx) {
	openid := string(ctx.QueryArgs().Peek("openid"))
	// 获取payment参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}

	if openid == "" || payment == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "missing required parameters"})
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

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.Header.Set("Content-Type", "application/json")
	json.NewEncoder(ctx).Encode(map[string]bool{"exists": exists})
}

// WechatAuth 微信公众号授权入口
func (ar *APIRoutes) WechatAuth(ctx *fasthttp.RequestCtx) {
	// 获取当前主机名
	host := string(ctx.Host())

	// 获取重定向URL参数
	redirectURL := string(ctx.QueryArgs().Peek("redirect_url"))

	// 获取payment和categories参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}
	categories := string(ctx.QueryArgs().Peek("categories"))
	if categories == "" {
		categories = string(ctx.QueryArgs().Peek("c"))
	}

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
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "failed to generate auth URL"})
		return
	}

	// 302重定向到微信授权页面
	ctx.Redirect(authURL, fasthttp.StatusFound)
}

// WechatAuthCallback 微信公众号授权回调处理
func (ar *APIRoutes) WechatAuthCallback(ctx *fasthttp.RequestCtx) {
	// 获取授权码
	code := string(ctx.QueryArgs().Peek("code"))

	// 获取重定向URL参数
	redirectURL := string(ctx.QueryArgs().Peek("redirect_url"))

	// 获取payment和categories参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}
	categories := string(ctx.QueryArgs().Peek("categories"))
	if categories == "" {
		categories = string(ctx.QueryArgs().Peek("c"))
	}

	// 构建重定向URL
	redirectURL = ar.buildRedirectURL(redirectURL, payment, categories)

	if code == "" {
		// 未获取到授权码，设置为匿名施主
		ar.setAnonymousWechatCookie(ctx)
		ctx.Redirect(redirectURL, fasthttp.StatusFound)
		return
	}

	// 使用授权码获取用户信息
	userInfo, err := ar.paymentService.GetWechatUserInfoByCode(code)
	if err != nil {
		// 授权失败，设置为匿名施主
		ar.setAnonymousWechatCookie(ctx)
		ctx.Redirect(redirectURL, fasthttp.StatusFound)
		return
	}

	// 将用户信息存储到cookie中，方便后续使用
	if openid, ok := userInfo["openid"].(string); ok {
		cookie := &fasthttp.Cookie{}
		cookie.SetKey("wechat_openid")
		cookie.SetValue(openid)
		cookie.SetMaxAge(86400)
		cookie.SetPath("/")
		ctx.Response.Header.SetCookie(cookie)

		cookie = &fasthttp.Cookie{}
		cookie.SetKey("wechat_user_id")
		cookie.SetValue(openid)
		cookie.SetMaxAge(86400)
		cookie.SetPath("/")
		ctx.Response.Header.SetCookie(cookie)
	}
	if nickname, ok := userInfo["nickname"].(string); ok {
		cookie := &fasthttp.Cookie{}
		cookie.SetKey("wechat_user_name")
		cookie.SetValue(url.QueryEscape(nickname))
		cookie.SetMaxAge(86400)
		cookie.SetPath("/")
		ctx.Response.Header.SetCookie(cookie)
	}
	if headimgurl, ok := userInfo["headimgurl"].(string); ok {
		cookie := &fasthttp.Cookie{}
		cookie.SetKey("wechat_avatar_url")
		cookie.SetValue(url.QueryEscape(headimgurl))
		cookie.SetMaxAge(86400)
		cookie.SetPath("/")
		ctx.Response.Header.SetCookie(cookie)
	}

	// 重定向回原页面，添加授权标记
	ctx.Redirect(redirectURL, fasthttp.StatusFound)
}

// AlipayAuth 支付宝授权入口
func (ar *APIRoutes) AlipayAuth(ctx *fasthttp.RequestCtx) {
	// 获取当前主机名
	host := string(ctx.Host())

	// 获取重定向URL参数
	redirectURL := string(ctx.QueryArgs().Peek("redirect_url"))

	// 获取payment和categories参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}
	categories := string(ctx.QueryArgs().Peek("categories"))
	if categories == "" {
		categories = string(ctx.QueryArgs().Peek("c"))
	}

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
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "failed to generate auth URL"})
		return
	}

	// 302重定向到支付宝授权页面
	ctx.Redirect(authURL, fasthttp.StatusFound)
}

// AlipayAuthCallback 支付宝授权回调处理
func (ar *APIRoutes) AlipayAuthCallback(ctx *fasthttp.RequestCtx) {
	// 获取授权码
	code := string(ctx.QueryArgs().Peek("auth_code"))

	// 从state参数中获取重定向URL
	redirectURL := string(ctx.QueryArgs().Peek("state"))
	// 解码state参数
	var err error
	if redirectURL != "" {
		redirectURL, err = url.QueryUnescape(redirectURL)
		if err != nil {
			redirectURL = ""
		}
	}

	// 获取payment和categories参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}
	categories := string(ctx.QueryArgs().Peek("categories"))
	if categories == "" {
		categories = string(ctx.QueryArgs().Peek("c"))
	}

	// 尝试从redirect_url中解析payment和categories参数（支持别名）
	if payment == "" || categories == "" {
		if redirectURL != "" {
			parsedURL, err := url.Parse(redirectURL)
			if err == nil {
				params := parsedURL.Query()
				if payment == "" {
					payment = params.Get("payment")
					if payment == "" {
						payment = params.Get("p")
					}
				}
				if categories == "" {
					categories = params.Get("categories")
					if categories == "" {
						categories = params.Get("c")
					}
				}
			}
		}
	}

	// 构建重定向URL
	redirectURL = ar.buildRedirectURL(redirectURL, payment, categories)

	if code == "" {
		// 未获取到授权码，设置为匿名施主
		ar.setAnonymousAlipayCookie(ctx)
		ctx.Redirect(redirectURL, fasthttp.StatusFound)
		return
	}

	// 使用授权码获取用户信息
	userInfo, err := ar.paymentService.GetAlipayUserInfoByCode(code)
	if err != nil {
		// 授权失败，设置为匿名施主
		ar.setAnonymousAlipayCookie(ctx)
		ctx.Redirect(redirectURL, fasthttp.StatusFound)
		return
	}

	// 将用户信息存储到cookie中，方便后续使用
	userID := userInfo["user_id"]
	userName := userInfo["user_name"]
	avatarURL := userInfo["avatar_url"]
	accessToken := userInfo["access_token"]

	// 对包含特殊字符的值进行URL编码，确保cookie存储正确
	cookie := &fasthttp.Cookie{}
	cookie.SetKey("alipay_user_id")
	cookie.SetValue(userID)
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("alipay_user_name")
	cookie.SetValue(url.QueryEscape(userName))
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("alipay_avatar_url")
	cookie.SetValue(url.QueryEscape(avatarURL))
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	// 保存access_token到cookie中，用于后续获取用户信息
	if accessToken != "" {
		cookie = &fasthttp.Cookie{}
		cookie.SetKey("alipay_access_token")
		cookie.SetValue(accessToken)
		cookie.SetMaxAge(86400)
		cookie.SetPath("/")
		ctx.Response.Header.SetCookie(cookie)

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
	ctx.Redirect(redirectURL, fasthttp.StatusFound)
}

// HandleCallback 处理支付回调（WAP支付方式）
func (ar *APIRoutes) HandleCallback(ctx *fasthttp.RequestCtx) {
	// 添加防缓存头
	ctx.Response.Header.Set("Cache-Control", "no-cache,no-store,must-revalidate")
	ctx.Response.Header.Set("Pragma", "no-cache")
	ctx.Response.Header.Set("Expires", "0")

	// 关闭压缩
	ctx.Response.Header.Del("Content-Encoding")

	// 读取请求体
	body := ctx.PostBody()

	// 记录完整的请求体（用于调试）
	log.Printf("WebHook request body: %s", string(body))

	// 解析JSON数据，使用map[string]interface{}处理数组字段
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("WebHook request unmarshal error: %v, IP=%s", err, string(ctx.RemoteIP().String()))
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("success")
		return
	}

	// 记录解析后的数据结构（用于调试）
	log.Printf("WebHook parsed data: %v", data)

	// 解析订单号、金额、状态
	orderID, _ := data["client_sn"].(string)
	// 尝试从其他字段获取订单号（支持微信支付）
	if orderID == "" {
		if id, ok := data["order_id"].(string); ok {
			orderID = id
			log.Printf("Got order ID from order_id: %s", orderID)
		} else if id, ok := data["out_trade_no"].(string); ok {
			orderID = id
			log.Printf("Got order ID from out_trade_no: %s", orderID)
		} else if id, ok := data["transaction_id"].(string); ok {
			orderID = id
			log.Printf("Got order ID from transaction_id: %s", orderID)
		}
	}
	amount, _ := data["amount"].(string)
	// 尝试从其他字段获取金额
	if amount == "" {
		if amt, ok := data["total_amount"].(string); ok {
			amount = amt
			log.Printf("Got amount from total_amount: %s", amount)
		} else if amt, ok := data["pay_amount"].(string); ok {
			amount = amt
			log.Printf("Got amount from pay_amount: %s", amount)
		}
	}
	status, _ := data["status"].(string)
	// 尝试从其他字段获取状态
	if status == "" {
		if stat, ok := data["trade_status"].(string); ok {
			status = stat
			log.Printf("Got status from trade_status: %s", status)
		} else if stat, ok := data["result_code"].(string); ok {
			status = stat
			log.Printf("Got status from result_code: %s", status)
		}
	}

	// 检查支付状态（支持多种状态格式）
	successStatuses := []string{"success", "SUCCESS", "TRADE_SUCCESS", "PAY_SUCCESS"}
	isSuccess := false
	for _, s := range successStatuses {
		if status == s {
			isSuccess = true
			break
		}
	}

	// 非成功状态直接返回success
	if !isSuccess {
		log.Printf("WebHook status not success: orderNo=%s, status=%s, IP=%s", orderID, status, string(ctx.RemoteIP().String()))
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.WriteString("success")
		return
	}

	// 标准化状态值
	status = "success"

	// 获取Authorization头中的sign
	auth := string(ctx.Request.Header.Peek("Authorization"))

	// 验签
	var verifyErr error
	if auth != "" {
		// 方式1：使用RSA公钥验证（推荐）
		verifyErr = ar.paymentService.HandleCallbackWithPublicKey(data, auth, body)
	} else if sign, ok := data["sign"].(string); ok && sign != "" {
		// 方式2：使用终端密钥验证（兼容旧版）
		verifyErr = ar.paymentService.HandleCallback(data)
	} else {
		log.Printf("WebHook missing sign: IP=%s", string(ctx.RemoteIP().String()))
		ctx.SetStatusCode(fasthttp.StatusForbidden)
		ctx.WriteString("missing sign")
		return
	}

	// 验签失败返403
	if verifyErr != nil {
		log.Printf("WebHook signature verify failed: orderNo=%s, IP=%s, err=%v", orderID, string(ctx.RemoteIP().String()), verifyErr)
		ctx.SetStatusCode(fasthttp.StatusForbidden)
		ctx.WriteString("signature verify failed")
		return
	}

	// 立即返回success（100ms内）
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.WriteString("success")

	// 异步处理DB更新和广播
	go func() {
		// 更新DB
		if err := ar.updateOrderStatusToPaid(orderID, amount); err != nil {
			log.Printf("Update order status failed: %v, orderNo=%s", err, orderID)
			return
		}

		// 广播支付成功消息
		notification := &PayNotification{
			Type:    "pay_success",
			OrderNo: orderID,
			Amount:  amount,
			Time:    utils.Now(),
		}

		// 尝试从订单或回调数据中获取支付方式和分类信息
		payment := ""
		categories := ""

		// 1. 首先从数据库获取订单信息，获取最准确的项目和分类
		// 重要：这里的payment是项目ID，不是支付方式
		// categories是分类ID，不是支付方式
		var donation models.Donation
		if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err == nil {
			if donation.PaymentConfigID != "" {
				payment = donation.PaymentConfigID // 使用订单的项目ID
				log.Printf("Got project ID from database: %s", payment)
			}
			if donation.Categories != "" {
				categories = donation.Categories // 使用订单的分类ID
				log.Printf("Got category ID from database: %s", categories)
			}
			// 同时获取支付类型（用于日志记录）
			if donation.Payment != "" {
				log.Printf("Got payment method from database: %s", donation.Payment)
			}
		}

		// 2. 尝试从数据中获取项目相关信息（如果数据库查询失败）
		if payment == "" {
			// 注意：这里应该获取项目ID，不是支付方式
			if projectID, ok := data["project_id"].(string); ok {
				payment = projectID
				log.Printf("Got project ID from data.project_id: %s", payment)
			} else if projectID, ok := data["project"].(string); ok {
				payment = projectID
				log.Printf("Got project ID from data.project: %s", payment)
			}
		}

		// 3. 尝试从数据中获取分类相关信息
		if categories == "" {
			if cat, ok := data["categories"].(string); ok {
				categories = cat
				log.Printf("Got categories from data.categories: %s", categories)
			} else if cat, ok := data["category"].(string); ok {
				categories = cat
				log.Printf("Got categories from data.category: %s", categories)
			} else if cat, ok := data["category_id"].(string); ok {
				categories = cat
				log.Printf("Got categories from data.category_id: %s", categories)
			} else if cat, ok := data["categoryId"].(string); ok {
				categories = cat
				log.Printf("Got categories from data.categoryId: %s", categories)
			}
		}

		// 4. 记录支付方式信息（用于日志）
		// 注意：不再基于支付方式设置广播目标参数
		// 而是直接使用订单的项目和分类ID
		if paymentMethod, ok := data["payment"].(string); ok {
			log.Printf("Payment method from callback: %s", paymentMethod)
		} else if paymentMethod, ok := data["payment_type"].(string); ok {
			log.Printf("Payment method from callback: %s", paymentMethod)
		}

		// 5. 检查是否是微信支付或支付宝回调（用于日志和广播控制）
		isWeChatPay := false
		isAlipay := false

		if wechatData, hasWechat := data["wechat"].(map[string]interface{}); hasWechat {
			isWeChatPay = true
			log.Printf("Detected WeChat Pay callback: orderNo=%s", orderID)
			log.Printf("WeChat Pay data: %v", wechatData)
			// 尝试从微信支付嵌套数据中获取信息
			if wxOrderID, ok := wechatData["order_id"].(string); ok && orderID == "" {
				orderID = wxOrderID
				log.Printf("Got order ID from wechat.order_id: %s", orderID)
			}
			if wxAmount, ok := wechatData["amount"].(string); ok && amount == "" {
				amount = wxAmount
				log.Printf("Got amount from wechat.amount: %s", amount)
			}
			if wxStatus, ok := wechatData["status"].(string); ok && status == "" {
				status = wxStatus
				log.Printf("Got status from wechat.status: %s", status)
			}
		} else if alipayData, hasAlipay := data["alipay"].(map[string]interface{}); hasAlipay {
			isAlipay = true
			log.Printf("Detected Alipay callback: orderNo=%s", orderID)
			log.Printf("Alipay data: %v", alipayData)
			// 尝试从支付宝嵌套数据中获取信息
			if aliOrderID, ok := alipayData["order_id"].(string); ok && orderID == "" {
				orderID = aliOrderID
				log.Printf("Got order ID from alipay.order_id: %s", orderID)
			}
			if aliAmount, ok := alipayData["amount"].(string); ok && amount == "" {
				amount = aliAmount
				log.Printf("Got amount from alipay.amount: %s", amount)
			}
			if aliStatus, ok := alipayData["status"].(string); ok && status == "" {
				status = aliStatus
				log.Printf("Got status from alipay.status: %s", status)
			}
		}

		// 6. 重要：直接使用订单的实际项目和分类参数
		// payment参数是项目ID，不是支付方式
		// categories参数是分类ID，不是支付方式
		// 从数据库获取的订单信息已经包含了正确的项目和分类ID
		// 移除基于支付方式的参数转换，直接使用订单的实际参数
		log.Printf("Using actual order parameters: payment=%s, categories=%s", payment, categories)

		// 7. 最终检查
		log.Printf("Final broadcast parameters: payment=%s, categories=%s", payment, categories)

		// 记录广播信息
		log.Printf("Preparing to broadcast payment notification: orderNo=%s, amount=%s, payment=%s, categories=%s, isWeChatPay=%t, isAlipay=%t", orderID, amount, payment, categories, isWeChatPay, isAlipay)

		// 只对支付宝进行广播，微信支付的广播由状态轮询处理
		if isAlipay {
			// 使用定向广播
			if payment != "" || categories != "" {
				// 定向广播到特定参数的客户端
				ar.wsManager.BroadcastToSpecific(notification, payment, categories)
				log.Printf("Sent targeted broadcast for Alipay: orderNo=%s, payment=%s, categories=%s", orderID, payment, categories)
			} else {
				// 如果没有参数，使用全局广播
				ar.wsManager.Broadcast(notification)
				log.Printf("Sent global broadcast for Alipay: orderNo=%s, amount=%s", orderID, amount)
			}
		} else if isWeChatPay {
			// 微信支付不在这里广播，由状态轮询处理
			log.Printf("Skipping broadcast for WeChat Pay, will be handled by status polling", orderID)
		} else {
			// 其他支付方式，使用默认广播
			if payment != "" || categories != "" {
				ar.wsManager.BroadcastToSpecific(notification, payment, categories)
				log.Printf("Sent targeted broadcast for other payment: orderNo=%s, payment=%s, categories=%s", orderID, payment, categories)
			} else {
				ar.wsManager.Broadcast(notification)
				log.Printf("Sent global broadcast for other payment: orderNo=%s, amount=%s", orderID, amount)
			}
		}
	}()
}

// updateOrderStatusToPaid 更新订单状态为已支付
// TODO: 生产必改点3：实现真实的数据库更新逻辑
func (ar *APIRoutes) updateOrderStatusToPaid(orderNo, amount string) error {
	// 短暂延迟，确保数据库事务已提交
	time.Sleep(1 * time.Second)

	// 获取与当前订单相关的捐款记录
	ar.paymentService.GetDonationByOrderID(orderNo)

	// 示例：更新订单状态
	// 真实场景需要连接数据库并执行更新操作
	log.Printf("Update order status to paid: orderNo=%s, amount=%s", orderNo, amount)

	return nil // 替换为真实数据库操作
}

// GetRankings 获取捐款排行榜
func (ar *APIRoutes) GetRankings(ctx *fasthttp.RequestCtx) {
	// 创建带超时的上下文，设置10秒超时
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 解析limit参数，设置默认值和范围校验
	limitStr := string(ctx.QueryArgs().Peek("limit"))
	if limitStr == "" {
		limitStr = "10"
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}
	// 限制最大返回数量，防止性能问题
	if limit > 100 {
		limit = 100
	}

	// 解析page参数，设置默认值和范围校验
	pageStr := string(ctx.QueryArgs().Peek("page"))
	if pageStr == "" {
		pageStr = "1"
	}
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}

	// 获取payment和categories参数（支持别名）
	paymentConfigID := string(ctx.QueryArgs().Peek("payment"))
	if paymentConfigID == "" {
		paymentConfigID = string(ctx.QueryArgs().Peek("p"))
	}
	categoryID := string(ctx.QueryArgs().Peek("categories"))
	if categoryID == "" {
		categoryID = string(ctx.QueryArgs().Peek("c"))
	}

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
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			ctx.Response.Header.Set("Content-Type", "application/json")
			json.NewEncoder(ctx).Encode(map[string]string{"error": res.err.Error()})
			return
		}

		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]interface{}{
			"rankings": res.rankings,
			"pagination": map[string]interface{}{
				"limit":  limit,
				"page":   page,
				"offset": offset,
				"total":  len(res.rankings),
			},
		})
	case <-ctxTimeout.Done():
		ctx.SetStatusCode(fasthttp.StatusRequestTimeout)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "请求超时，请稍后再试"})
		return
	}
}

// ActivateTerminal 手动激活终端API
func (ar *APIRoutes) ActivateTerminal(ctx *fasthttp.RequestCtx) {
	// 从请求体获取激活码
	var req struct {
		ActivationCode string `json:"activation_code"`
	}

	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": err.Error()})
		return
	}

	// 执行终端激活
	if err := ar.paymentService.ActivateTerminal(req.ActivationCode); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{
			"error": fmt.Sprintf("Terminal activation failed: %v", err),
		})
		return
	}

	// 获取激活后的终端配置
	config := ar.paymentService.Config()

	// 返回成功响应
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.Header.Set("Content-Type", "application/json")
	json.NewEncoder(ctx).Encode(map[string]string{
		"message":      "Terminal activation successful",
		"terminal_sn":  config.TerminalSN,
		"terminal_key": config.TerminalKey,
	})
}

// GenerateQRCode 生成统一支付二维码
func (ar *APIRoutes) GenerateQRCode(ctx *fasthttp.RequestCtx) {
	// 获取payment参数（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}

	// 如果payment参数不存在，返回首页
	if payment == "" {
		ctx.Redirect("/", fasthttp.StatusFound)
		return
	}

	// 获取categories参数（支持别名）
	categories := string(ctx.QueryArgs().Peek("categories"))
	if categories == "" {
		categories = string(ctx.QueryArgs().Peek("c"))
	}

	// 当payment有参数时，如果没有categories参数，自动设置默认的categories参数
	if categories == "" {
		// 设置默认的categories参数为 "1"
		categories = "1"
	}

	// 获取请求的主机名
	host := string(ctx.Host())

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
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": err.Error()})
		return
	}

	ctx.Response.Header.Set("Content-Type", "image/png")
	ctx.Write(qrBytes)
}

// GetPaymentConfig 获取支付配置信息
func (ar *APIRoutes) GetPaymentConfig(ctx *fasthttp.RequestCtx) {
	// 从路径中获取ID参数
	path := string(ctx.Path())
	id := path[len("/api/payment-config/"):]
	if id == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "缺少支付配置ID参数"})
		return
	}

	var paymentConfig models.PaymentConfig
	if err := utils.DB.Where("id = ?", id).First(&paymentConfig).Error; err != nil {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "支付配置不存在"})
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(ctx).Encode(paymentConfig)
}

// GetCategory 获取类目信息
func (ar *APIRoutes) GetCategory(ctx *fasthttp.RequestCtx) {
	// 从路径中获取ID参数
	path := string(ctx.Path())
	id := path[len("/api/category/"):]
	if id == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "缺少类目ID参数"})
		return
	}

	var category models.Category
	if err := utils.DB.Where("id = ?", id).First(&category).Error; err != nil {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "类目不存在"})
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(ctx).Encode(category)
}

// GetCategories 获取所有类目列表
func (ar *APIRoutes) GetCategories(ctx *fasthttp.RequestCtx) {
	var categories []models.Category
	query := utils.DB

	// 根据payment参数过滤（支持别名）
	payment := string(ctx.QueryArgs().Peek("payment"))
	if payment == "" {
		payment = string(ctx.QueryArgs().Peek("p"))
	}
	if payment != "" {
		query = query.Where("payment = ?", payment)
	}

	if err := query.Find(&categories).Error; err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.Response.Header.Set("Content-Type", "application/json")
		json.NewEncoder(ctx).Encode(map[string]string{"error": "获取类目列表失败"})
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.Header.Set("Content-Type", "application/json")
	json.NewEncoder(ctx).Encode(categories)
}

// setAnonymousWechatCookie 设置微信匿名用户cookie
func (ar *APIRoutes) setAnonymousWechatCookie(ctx *fasthttp.RequestCtx) {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey("wechat_openid")
	cookie.SetValue("anonymous")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("wechat_user_id")
	cookie.SetValue("anonymous")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("wechat_user_name")
	cookie.SetValue("匿名施主")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("wechat_avatar_url")
	cookie.SetValue("./static/avatar.jpeg")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)
}

// setAnonymousAlipayCookie 设置支付宝匿名用户cookie
func (ar *APIRoutes) setAnonymousAlipayCookie(ctx *fasthttp.RequestCtx) {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey("alipay_user_id")
	cookie.SetValue("anonymous")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("alipay_user_name")
	cookie.SetValue("匿名施主")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)

	cookie = &fasthttp.Cookie{}
	cookie.SetKey("alipay_avatar_url")
	cookie.SetValue("./static/avatar.jpeg")
	cookie.SetMaxAge(86400)
	cookie.SetPath("/")
	ctx.Response.Header.SetCookie(cookie)
}

// buildRedirectURL 构建重定向URL
func (ar *APIRoutes) buildRedirectURL(redirectURL, payment, categories string) string {
	if redirectURL == "" {
		redirectURL = "/pay"

		if payment != "" {
			redirectURL += fmt.Sprintf("?payment=%s", payment)
			if categories != "" {
				redirectURL += fmt.Sprintf("&categories=%s", categories)
			}
		} else if categories != "" {
			redirectURL += fmt.Sprintf("?categories=%s", categories)
		}
	}

	if !strings.Contains(redirectURL, "authorized=") {
		if !strings.Contains(redirectURL, "?") {
			redirectURL += "?authorized=1"
		} else {
			redirectURL += "&authorized=1"
		}
	}

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

	return redirectURL
}
