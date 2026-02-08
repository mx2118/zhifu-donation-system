package services

import (
	"bytes"
	"crypto"
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhifu/donation-rank/models"
	"github.com/zhifu/donation-rank/utils"
)

// 初始化随机数生成器
func init() {
	// 使用当前时间作为随机数种子
	rand.Seed(time.Now().UnixNano())
}

type ShouqianbaConfig struct {
	// 开发者配置
	VendorSN  string
	VendorKey string
	AppID     string

	// 终端配置
	TerminalSN  string
	TerminalKey string

	// 设备配置
	DeviceID string

	// 业务配置
	MerchantID string
	StoreID    string
	StoreName  string

	// API配置
	APIURL     string
	GatewayURL string

	// 微信公众号配置
	WechatAppID     string
	WechatAppSecret string

	// 支付宝配置
	AlipayAppID      string
	AlipayPublicKey  string // 支付宝公钥，用于验证响应
	AlipayPrivateKey string // 应用私钥，用于请求签名
	AlipayGatewayURL string // 支付宝网关地址，如：https://openapi.alipay.com/gateway.do
	AlipayFormat     string // 请求格式，固定值json
	AlipayCharset    string // 字符集，如：utf-8
	AlipaySignType   string // 签名类型，如：RSA2
}

// AccessTokenInfo 微信access_token缓存信息
type AccessTokenInfo struct {
	AccessToken string
	ExpiresAt   time.Time
}

// PaymentService 支付服务
type PaymentService struct {
	config         ShouqianbaConfig
	lastSignInDate string          // 上次签到日期，格式：2006-01-02
	accessToken    AccessTokenInfo // 微信access_token缓存
	configCache    map[string]ShouqianbaConfig
	// 新增缓存字段
	rankingsCache       map[string][]RankingItem // 排行榜缓存，key为：paymentConfigID_categoryID_limit_offset
	latestDonationCache *RankingItem             // 最新捐款缓存
	cacheMutex          sync.RWMutex             // 缓存读写锁
	cacheExpiration     time.Duration            // 缓存过期时间
	// HTTP客户端连接池
	httpClient *http.Client
	// 广播状态管理
	BroadcastedOrders sync.Map // 已广播的订单，key为orderID，value为true
}

// Config 获取当前支付服务配置
func (ps *PaymentService) Config() ShouqianbaConfig {
	return ps.config
}

// isBroadcasted 检查订单是否已经广播过
func (ps *PaymentService) isBroadcasted(orderID string) bool {
	_, ok := ps.BroadcastedOrders.Load(orderID)
	return ok
}

// markAsBroadcasted 标记订单为已广播
func (ps *PaymentService) markAsBroadcasted(orderID string) {
	ps.BroadcastedOrders.Store(orderID, true)
}

// 注意：已删除LoadTerminalFromDB方法，现在配置从PaymentConfig表统一加载

func NewPaymentService(config ShouqianbaConfig) *PaymentService {
	// 创建HTTP客户端连接池
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 30 * time.Second,
	}

	return &PaymentService{
		config:         config,
		lastSignInDate: "", // 初始化时为空，第一次调用会触发签到
		configCache:    make(map[string]ShouqianbaConfig),
		// 初始化新增字段
		rankingsCache:       make(map[string][]RankingItem),
		latestDonationCache: nil,
		cacheMutex:          sync.RWMutex{},
		cacheExpiration:     5 * time.Minute, // 缓存5分钟
		httpClient:          httpClient,
	}
}

// GenerateSign 生成签名（严格按照跳转支付接口文档要求）
// 文档规则：
// 1. 筛选：获取所有请求参数，剔除sign与sign_type参数
// 2. 排序：按ASCII码递增排序
// 3. 拼接：参数=参数值&参数=参数值
// 4. 拼接密钥：&key=密钥值
// 5. MD5加密
// 6. 转大写
func (ps *PaymentService) GenerateSign(params map[string]string, signType string) string {
	// 1. 筛选参数：过滤空值，排除sign和sign_type参数
	filteredParams := make(map[string]string)
	for k, v := range params {
		// 排除空值和不需要的字段
		if v != "" && k != "sign" && k != "sign_type" && k != "flowT" && k != "flowSign" && k != "flow" {
			filteredParams[k] = v
		}
	}

	// 2. 按key的ASCII码升序排序
	keys := make([]string, 0, len(filteredParams))
	for k := range filteredParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 3. 拼接字符串：key=value&key=value格式
	var signStr strings.Builder
	for i, k := range keys {
		value := filteredParams[k]
		signStr.WriteString(fmt.Sprintf("%s=%s", k, value))
		if i < len(keys)-1 {
			signStr.WriteString("&")
		}
	}

	// 4. 拼接密钥，按照文档要求添加"&key="前缀
	var signKey string
	switch signType {
	case "terminal":
		// 使用终端密钥
		signKey = ps.config.TerminalKey
	case "vendor":
		// 使用开发者密钥
		signKey = ps.config.VendorKey
	default:
		// 默认使用开发者密钥
		signKey = ps.config.VendorKey
	}
	signStr.WriteString(fmt.Sprintf("&key=%s", signKey))
	signString := signStr.String()

	// 5. MD5加密，生成32位大写（按照文档要求）
	md5Hash := md5.Sum([]byte(signString))
	finalSign := strings.ToUpper(hex.EncodeToString(md5Hash[:]))

	return finalSign
}

// ActivateTerminal 终端激活，获取terminal_sn和terminal_key
func (ps *PaymentService) ActivateTerminal(code string) error {
	// 构建激活请求参数
	params := map[string]interface{}{
		"app_id":    ps.config.AppID,
		"code":      code,
		"device_id": ps.config.DeviceID, // 使用配置文件中的固定device_id
	}

	// 转换为JSON字符串
	jsonParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %v", err)
	}

	// 生成签名（JSON字符串 + 密钥）
	signStr := string(jsonParams) + ps.config.VendorKey
	md5Hash := md5.Sum([]byte(signStr))
	sign := hex.EncodeToString(md5Hash[:])

	// 构建请求URL
	url := fmt.Sprintf("%s/terminal/activate", ps.config.APIURL)

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonParams))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Format", "json")
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", ps.config.VendorSN, sign))

	// 发送请求
	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	fmt.Printf("Activate response: %s\n", body)

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode response: %v, response body: %s", err, body)
	}

	// 处理响应
	message, _ := result["message"].(string)
	if message == "Not Found" {
		return fmt.Errorf("API endpoint not found, response: %s", body)
	}

	// 处理业务响应
	if resultCode, ok := result["result_code"].(string); ok && resultCode != "SUCCESS" && resultCode != "200" {
		errMsg := "unknown error"
		if msg, ok := result["error_message"].(string); ok {
			errMsg = msg
		} else if msg, ok := result["err_msg"].(string); ok {
			errMsg = msg
		}
		return fmt.Errorf("activate terminal failed: %s, response: %s", errMsg, body)
	}

	// 更新终端配置
	var data map[string]interface{}
	if d, ok := result["data"].(map[string]interface{}); ok {
		data = d
	} else if d, ok := result["biz_response"].(map[string]interface{}); ok {
		data = d
	}

	if data != nil {
		if terminalSN, ok := data["terminal_sn"].(string); ok && terminalSN != "" {
			ps.config.TerminalSN = terminalSN
		}
		if terminalKey, ok := data["terminal_key"].(string); ok && terminalKey != "" {
			ps.config.TerminalKey = terminalKey
		}
		if merchantSN, ok := data["merchant_sn"].(string); ok {
			ps.config.MerchantID = merchantSN
		}
		if storeSN, ok := data["store_sn"].(string); ok {
			ps.config.StoreID = storeSN
		}
	}

	return nil
}

// SignIn 终端签到，更新terminal_key
func (ps *PaymentService) SignIn() error {
	// 检查终端配置是否已设置
	if ps.config.TerminalSN == "" || ps.config.TerminalKey == "" {
		return fmt.Errorf("terminal not activated")
	}

	// 构建签到请求参数
	params := map[string]interface{}{
		"terminal_sn": ps.config.TerminalSN,
		"device_id":   ps.config.DeviceID, // 使用配置文件中的固定device_id
	}

	// 转换为JSON字符串
	jsonParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %v", err)
	}

	// 生成签名（JSON字符串 + 终端密钥）
	signStr := string(jsonParams) + ps.config.TerminalKey
	md5Hash := md5.Sum([]byte(signStr))
	sign := hex.EncodeToString(md5Hash[:])

	// 构建请求URL，使用正确的checkin端点
	url := fmt.Sprintf("%s/terminal/checkin", ps.config.APIURL)

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonParams))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Format", "json")
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", ps.config.TerminalSN, sign))

	// 发送请求
	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	fmt.Printf("SignIn response: %s\n", body)

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode response: %v, response body: %s", err, body)
	}

	// 处理响应
	message, _ := result["message"].(string)
	if message == "Not Found" {
		return fmt.Errorf("API endpoint not found, response: %s", body)
	}

	// 处理业务响应
	resultCode, _ := result["result_code"].(string)
	if resultCode != "SUCCESS" && resultCode != "200" {
		errMsg := "unknown error"
		if msg, ok := result["error_message"].(string); ok {
			errMsg = msg
		} else if msg, ok := result["err_msg"].(string); ok {
			errMsg = msg
		}
		return fmt.Errorf("sign in failed: %s, response: %s", errMsg, body)
	}

	// 解析终端信息
	updated := false
	newTerminalKey := ps.config.TerminalKey
	newTerminalSN := ps.config.TerminalSN
	merchantSN := ""
	merchantName := ""
	storeSN := ""
	storeName := ""

	// 从响应中获取终端信息（支持两种响应格式：data或biz_response）
	var data map[string]interface{}
	if d, ok := result["data"].(map[string]interface{}); ok {
		data = d
	} else if d, ok := result["biz_response"].(map[string]interface{}); ok {
		data = d
	}

	if data != nil {
		if terminalKey, ok := data["terminal_key"].(string); ok && terminalKey != "" {
			newTerminalKey = terminalKey
			updated = true
		}
		if terminalSN, ok := data["terminal_sn"].(string); ok && terminalSN != "" {
			newTerminalSN = terminalSN
			updated = true
		}
		if msn, ok := data["merchant_sn"].(string); ok {
			merchantSN = msn
		}
		if mname, ok := data["merchant_name"].(string); ok {
			merchantName = mname
		}
		if ssn, ok := data["store_sn"].(string); ok {
			storeSN = ssn
		}
		if sname, ok := data["store_name"].(string); ok {
			storeName = sname
		}
	}

	// 如果终端配置有更新，更新内存中的配置
	if updated {
		ps.config.TerminalSN = newTerminalSN
		ps.config.TerminalKey = newTerminalKey
	}

	// 保存支付配置信息到数据库
	paymentConfig := models.PaymentConfig{
		VendorSN:     ps.config.VendorSN,
		VendorKey:    ps.config.VendorKey,
		AppID:        ps.config.AppID,
		TerminalSN:   newTerminalSN,
		TerminalKey:  newTerminalKey,
		MerchantSN:   merchantSN,
		MerchantName: merchantName,
		StoreSN:      storeSN,
		StoreName:    storeName,
		DeviceID:     ps.config.DeviceID,
		APIURL:       ps.config.APIURL,
		GatewayURL:   ps.config.GatewayURL,
		MerchantID:   ps.config.MerchantID,
		StoreID:      ps.config.StoreID,
		IsActive:     true,
		LastSignInAt: time.Now(),
	}

	// 使用utils.DB来保存支付配置信息
	if err := utils.DB.Where("terminal_sn = ?", newTerminalSN).Assign(paymentConfig).FirstOrCreate(&paymentConfig).Error; err != nil {
		log.Printf("Failed to save payment config to database: %v", err)
	}

	return nil
}

// QueryOrder 查询订单状态
func (ps *PaymentService) QueryOrder(orderID string) (map[string]interface{}, error) {
	// 首先查询订单，获取PaymentConfigID
	var donation models.Donation
	if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err != nil {
		return nil, fmt.Errorf("order not found: %v", err)
	}

	// 根据PaymentConfigID加载对应的配置
	var currentConfig ShouqianbaConfig
	if donation.PaymentConfigID != "" {
		// 尝试从缓存获取
		if cachedConfig, exists := ps.configCache[donation.PaymentConfigID]; exists {
			currentConfig = cachedConfig
			log.Printf("DEBUG: Using cached config for paymentConfigID=%s", donation.PaymentConfigID)
		} else {
			// 从数据库加载
			var dbConfig models.PaymentConfig
			if err := utils.DB.Where("id = ?", donation.PaymentConfigID).First(&dbConfig).Error; err != nil {
				log.Printf("Warning: Config with id=%s not found, using default config: %v", donation.PaymentConfigID, err)
				currentConfig = ps.config
			} else {
				// 转换为ShouqianbaConfig
				currentConfig = ShouqianbaConfig{
					VendorSN:         dbConfig.VendorSN,
					VendorKey:        dbConfig.VendorKey,
					AppID:            dbConfig.AppID,
					TerminalSN:       dbConfig.TerminalSN,
					TerminalKey:      dbConfig.TerminalKey,
					DeviceID:         dbConfig.DeviceID,
					MerchantID:       dbConfig.MerchantID,
					StoreID:          dbConfig.StoreID,
					StoreName:        dbConfig.StoreName,
					APIURL:           dbConfig.APIURL,
					GatewayURL:       dbConfig.GatewayURL,
					WechatAppID:      dbConfig.WechatAppID,
					WechatAppSecret:  dbConfig.WechatAppSecret,
					AlipayAppID:      dbConfig.AlipayAppID,
					AlipayPublicKey:  dbConfig.AlipayPublicKey,
					AlipayPrivateKey: dbConfig.AlipayPrivateKey,
				}
				// 缓存配置
				ps.configCache[donation.PaymentConfigID] = currentConfig
				log.Printf("DEBUG: Loaded config from database for paymentConfigID=%s, terminal_sn=%s", donation.PaymentConfigID, currentConfig.TerminalSN)
			}
		}
	} else {
		// 使用默认配置
		currentConfig = ps.config
		log.Printf("DEBUG: Using default config, terminal_sn=%s, store_name=%s", currentConfig.TerminalSN, currentConfig.StoreName)
	}

	// 检查终端配置是否已设置
	if currentConfig.TerminalSN == "" || currentConfig.TerminalKey == "" {
		return nil, fmt.Errorf("terminal not activated")
	}

	// 构建查询请求参数
	params := map[string]interface{}{
		"terminal_sn": currentConfig.TerminalSN,
		"client_sn":   orderID,
	}

	// 转换为JSON字符串
	jsonParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %v", err)
	}

	// 生成签名（JSON字符串 + 终端密钥）
	signStr := string(jsonParams) + currentConfig.TerminalKey
	md5Hash := md5.Sum([]byte(signStr))
	sign := hex.EncodeToString(md5Hash[:])

	// 构建请求URL
	url := fmt.Sprintf("%s/upay/v2/query", currentConfig.APIURL)

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonParams))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Format", "json")
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", currentConfig.TerminalSN, sign))

	// 发送请求
	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}
	fmt.Printf("QueryOrder response: %s\n", body)

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v, response body: %s", err, body)
	}

	// 处理响应
	resultCode, _ := result["result_code"].(string)
	// 主result_code可能是"200"或"SUCCESS"，需要同时处理这两种情况
	if resultCode != "SUCCESS" && resultCode != "200" {
		errMsg := "unknown error"
		if msg, ok := result["error_message"].(string); ok {
			errMsg = msg
		} else if msg, ok := result["err_msg"].(string); ok {
			errMsg = msg
		}
		return nil, fmt.Errorf("query order failed: %s, response: %s", errMsg, body)
	}

	return result, nil
}

// RefundOrder 退款订单
func (ps *PaymentService) RefundOrder(orderID string, amount float64) error {
	// 检查终端配置是否已设置
	if ps.config.TerminalSN == "" || ps.config.TerminalKey == "" {
		return fmt.Errorf("terminal not activated")
	}

	// 构建退款请求参数
	params := map[string]interface{}{
		"terminal_sn":    ps.config.TerminalSN,
		"client_sn":      fmt.Sprintf("REFUND%s", time.Now().Format("20060102150405")),
		"orig_client_sn": orderID,
		"refund_amount":  fmt.Sprintf("%.0f", amount*100), // 分
		"operator":       "donation_system",
	}

	// 转换为JSON字符串
	jsonParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %v", err)
	}

	// 生成签名（JSON字符串 + 终端密钥）
	signStr := string(jsonParams) + ps.config.TerminalKey
	md5Hash := md5.Sum([]byte(signStr))
	sign := hex.EncodeToString(md5Hash[:])

	// 构建请求URL
	url := fmt.Sprintf("%s/upay/v2/refund", ps.config.APIURL)

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonParams))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Format", "json")
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", ps.config.TerminalSN, sign))

	// 发送请求
	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}
	fmt.Printf("RefundOrder response: %s\n", body)

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode response: %v, response body: %s", err, body)
	}

	// 处理响应
	if resultCode, ok := result["result_code"].(string); ok && resultCode != "SUCCESS" {
		errMsg := "unknown error"
		if msg, ok := result["error_message"].(string); ok {
			errMsg = msg
		} else if msg, ok := result["err_msg"].(string); ok {
			errMsg = msg
		}
		return fmt.Errorf("refund order failed: %s, response: %s", errMsg, body)
	}

	return nil
}

// CreateOrder 创建支付订单（WAP支付方式）
// CreateOrder 创建支付订单
// host: 当前请求的主机名（例如：192.168.19.52:9090 或 101.34.24.139:9090）
// openid: 微信用户的openid（可选，已授权用户提供）
// paymentConfigID: 支付配置ID// CreateOrder 创建捐款订单
func (ps *PaymentService) CreateOrder(amount float64, payment string, host string, openid string, categoryID string, paymentConfigID string, blessing string) (string, string, error) {
	// 根据paymentConfigID加载对应的配置
	var currentConfig ShouqianbaConfig
	if paymentConfigID != "" {
		// 尝试从缓存获取
		if cachedConfig, exists := ps.configCache[paymentConfigID]; exists {
			// 检查缓存配置是否包含StoreName字段
			if cachedConfig.StoreName == "" {
				// 缓存配置缺少StoreName，从数据库重新加载
				log.Printf("DEBUG: Cached config missing StoreName, reloading from database for paymentConfigID=%s", paymentConfigID)
				// 从数据库加载
				var dbConfig models.PaymentConfig
				if err := utils.DB.Where("id = ?", paymentConfigID).First(&dbConfig).Error; err != nil {
					log.Printf("Warning: Config with id=%s not found, using default config: %v", paymentConfigID, err)
					currentConfig = ps.config
				} else {
					// 转换为ShouqianbaConfig
					currentConfig = ShouqianbaConfig{
						VendorSN:         dbConfig.VendorSN,
						VendorKey:        dbConfig.VendorKey,
						AppID:            dbConfig.AppID,
						TerminalSN:       dbConfig.TerminalSN,
						TerminalKey:      dbConfig.TerminalKey,
						DeviceID:         dbConfig.DeviceID,
						MerchantID:       dbConfig.MerchantID,
						StoreID:          dbConfig.StoreID,
						StoreName:        dbConfig.StoreName,
						APIURL:           dbConfig.APIURL,
						GatewayURL:       dbConfig.GatewayURL,
						WechatAppID:      dbConfig.WechatAppID,
						WechatAppSecret:  dbConfig.WechatAppSecret,
						AlipayAppID:      dbConfig.AlipayAppID,
						AlipayPublicKey:  dbConfig.AlipayPublicKey,
						AlipayPrivateKey: dbConfig.AlipayPrivateKey,
					}
					// 更新缓存
					ps.configCache[paymentConfigID] = currentConfig
					log.Printf("DEBUG: Reloaded config from database for paymentConfigID=%s, terminal_sn=%s, store_name=%s", paymentConfigID, currentConfig.TerminalSN, currentConfig.StoreName)
				}
			} else {
				currentConfig = cachedConfig
				log.Printf("DEBUG: Using cached config for paymentConfigID=%s, store_name=%s", paymentConfigID, currentConfig.StoreName)
			}
		} else {
			// 从数据库加载
			var dbConfig models.PaymentConfig
			if err := utils.DB.Where("id = ?", paymentConfigID).First(&dbConfig).Error; err != nil {
				log.Printf("Warning: Config with id=%s not found, using default config: %v", paymentConfigID, err)
				currentConfig = ps.config
			} else {
				// 转换为ShouqianbaConfig
				currentConfig = ShouqianbaConfig{
					VendorSN:         dbConfig.VendorSN,
					VendorKey:        dbConfig.VendorKey,
					AppID:            dbConfig.AppID,
					TerminalSN:       dbConfig.TerminalSN,
					TerminalKey:      dbConfig.TerminalKey,
					DeviceID:         dbConfig.DeviceID,
					MerchantID:       dbConfig.MerchantID,
					StoreID:          dbConfig.StoreID,
					StoreName:        dbConfig.StoreName,
					APIURL:           dbConfig.APIURL,
					GatewayURL:       dbConfig.GatewayURL,
					WechatAppID:      dbConfig.WechatAppID,
					WechatAppSecret:  dbConfig.WechatAppSecret,
					AlipayAppID:      dbConfig.AlipayAppID,
					AlipayPublicKey:  dbConfig.AlipayPublicKey,
					AlipayPrivateKey: dbConfig.AlipayPrivateKey,
				}
				// 缓存配置
				ps.configCache[paymentConfigID] = currentConfig
				log.Printf("DEBUG: Loaded config from database for paymentConfigID=%s, terminal_sn=%s, store_name=%s", paymentConfigID, currentConfig.TerminalSN, currentConfig.StoreName)
			}
		}
	} else {
		// 使用默认配置
		currentConfig = ps.config
		log.Printf("DEBUG: Using default config, terminal_sn=%s", currentConfig.TerminalSN)
	}

	// 为当前配置执行签到
	currentDate := time.Now().Format("2006-01-02")
	if ps.lastSignInDate != currentDate {
		// 保存原始配置
		originalConfig := ps.config
		// 使用当前配置进行签到
		ps.config = currentConfig
		if err := ps.SignIn(); err != nil {
			// 签到失败不阻止订单创建，继续使用当前终端密钥
			log.Printf("Warning: Sign-in failed for config %s: %v", paymentConfigID, err)
		} else {
			// 签到成功，更新上次签到日期
			ps.lastSignInDate = currentDate
			// 更新缓存中的配置
			if paymentConfigID != "" {
				ps.configCache[paymentConfigID] = ps.config
			}
		}
		// 恢复原始配置
		ps.config = originalConfig
	}

	// 参数验证
	// 1. 金额验证：检查金额是否在合理范围内（0.01元到10000元）
	if amount < 0.01 || amount > 10000 {
		return "", "", fmt.Errorf("amount must be between 0.01 and 10000")
	}

	// 2. 生成商户系统订单号：使用时间+随机数确保唯一性
	orderID := fmt.Sprintf("ORD%s%04d", time.Now().Format("20060102150405"), rand.Intn(10000))

	// 3. 订单号长度验证：确保不超过64字节
	if len(orderID) > 64 {
		return "", "", fmt.Errorf("order_id too long, must be less than 64 bytes")
	}

	// 4. 确保金额转换为分单位后至少为1分（使用四舍五入，避免截断问题）
	totalAmount := int64(math.Round(amount * 100))
	if totalAmount < 1 {
		totalAmount = 1
	}

	// 基础URL
	baseURL := currentConfig.GatewayURL

	// 回调和返回URL
	notifyURL := fmt.Sprintf("http://%s/api/callback", host)
	// 构建返回URL，包含payment和category参数，直接跳转到首页（功德榜）
	returnURL := fmt.Sprintf("http://%s", host)
	if paymentConfigID != "" {
		returnURL += fmt.Sprintf("?payment=%s", paymentConfigID)
		if categoryID != "" {
			returnURL += fmt.Sprintf("&categories=%s", categoryID)
		}
	} else if categoryID != "" {
		returnURL += fmt.Sprintf("?categories=%s", categoryID)
	}

	// 验证支付方式
	if payment != "wechat" && payment != "alipay" {
		return "", "", fmt.Errorf("invalid payment type: %s", payment)
	}

	// 构建WAP支付请求参数（严格按照WAP2文档要求，只包含必要参数）
	// 根据支付类型设置不同的payway值（根据官方文档修正取值）
	var payway string
	// 直接使用if-else语句，避免switch语句的潜在问题
	if payment == "wechat" {
		payway = "3" // 微信支付（正确取值：3）
	} else if payment == "alipay" {
		payway = "1" // 支付宝支付（正确取值：1）
	} else {
		payway = "3" // 默认微信支付
	}

	// 根据categoryID查询Category表，获取产品名称
	categoryName := ""
	if categoryID != "" {
		var category models.Category
		// 直接使用字符串ID查询，GORM会自动处理类型转换
		if err := utils.DB.Where("id = ?", categoryID).First(&category).Error; err == nil {
			categoryName = category.Name
		}
		// 如果查询失败，尝试将字符串转换为uint后查询
		if categoryName == "" {
			if categoryIDUint, err := strconv.ParseUint(categoryID, 10, 32); err == nil {
				if err := utils.DB.Where("id = ?", uint(categoryIDUint)).First(&category).Error; err == nil {
					categoryName = category.Name
				}
			}
		}
	}

	// 根据捐款类目设置交易概述
	log.Printf("DEBUG: StoreName value: '%s'", currentConfig.StoreName)
	log.Printf("DEBUG: CategoryName value: '%s'", categoryName)
	subject := "捐款"
	if currentConfig.StoreName != "" {
		log.Printf("DEBUG: Using StoreName: '%s'", currentConfig.StoreName)
		subject = "捐款-" + currentConfig.StoreName
		if categoryName != "" {
			log.Printf("DEBUG: Using CategoryName: '%s'", categoryName)
			subject += "-" + categoryName
		}
	} else if categoryName != "" {
		log.Printf("DEBUG: Only using CategoryName: '%s'", categoryName)
		subject = "捐款-" + categoryName
	} else {
		log.Printf("DEBUG: Using default subject: '捐款'\n")
	}
	log.Printf("DEBUG: Generated subject: '%s'", subject)
	// 确保subject参数的长度不超过支付网关的限制
	if len(subject) > 50 {
		subject = subject[:50]
		log.Printf("DEBUG: Truncated subject to 50 chars: '%s'", subject)
	}

	// 调整参数顺序，将payway和reflect放在前面，确保支付方式优先被识别
	// 构建备注信息，格式为：store_name-category
	reflectText := ""
	if currentConfig.StoreName != "" && categoryName != "" {
		reflectText = fmt.Sprintf("%s-%s", currentConfig.StoreName, categoryName)
	} else if currentConfig.StoreName != "" {
		reflectText = currentConfig.StoreName
	} else if categoryName != "" {
		reflectText = categoryName
	} else {
		reflectText = "捐款"
	}

	params := map[string]string{
		"payway":       payway,                         // 支付方式（必填，优先设置）
		"reflect":      reflectText,                    // 反射参数（必填，格式：store_name-category）
		"terminal_sn":  currentConfig.TerminalSN,       // 收钱吧终端ID（必填）
		"client_sn":    orderID,                        // 商户系统订单号（必填）
		"total_amount": fmt.Sprintf("%d", totalAmount), // 交易总金额（分，必填）
		"subject":      subject,                        // 交易概述（必填）
		"operator":     "donation_system",              // 门店操作员（必填）
		"return_url":   returnURL,                      // 页面跳转同步通知页面路径（必填）
		"notify_url":   notifyURL,                      // 服务器异步回调url（选填）
	}

	// 临时保存原始配置，使用当前配置生成签名
	originalConfig := ps.config
	ps.config = currentConfig
	// 根据收钱吧API文档，跳转支付接口（WAP支付）应该使用终端密钥（terminal_key）
	sign := ps.GenerateSign(params, "terminal")
	// 恢复原始配置
	ps.config = originalConfig

	// 添加签名到参数
	params["sign"] = sign

	// 构建完整的网关URL（签名值不进行URL编码）
	// 按特定顺序排序参数，确保payway和reflect优先，并且签名生成与URL构建使用相同顺序
	paramOrder := []string{
		"payway",
		"reflect",
		"terminal_sn",
		"client_sn",
		"total_amount",
		"subject",
		"operator",
		"return_url",
		"notify_url",
		"sign",
	}

	var queryBuilder strings.Builder
	for _, k := range paramOrder {
		if v, exists := params[k]; exists {
			key := url.QueryEscape(k)
			var val string
			if k == "sign" {
				// 签名值不进行URL编码
				val = v
			} else {
				// 其他参数值进行URL编码
				val = url.QueryEscape(v)
			}
			queryBuilder.WriteString(fmt.Sprintf("%s=%s&", key, val))
		}
	}
	queryStr := strings.TrimSuffix(queryBuilder.String(), "&")
	payURL := fmt.Sprintf("%s?%s", baseURL, queryStr)

	// 保存订单
	// 初始化订单信息
	userID := fmt.Sprintf("TEMP_%d", time.Now().UnixNano())

	// 如果提供了openid，尝试从数据库获取用户信息
	if openid != "" {

		// 根据支付类型查询不同的用户表
		if payment == "wechat" {
			// 微信用户，查询微信用户表
			var wechatUser models.WechatUser
			if err := utils.DB.Where(&models.WechatUser{OpenID: openid}).First(&wechatUser).Error; err == nil {
				// 找到用户信息，使用真实信息
				userID = wechatUser.OpenID
				log.Printf("DEBUG: Found wechat user info, using real openid as user_id: %s", userID)
				// 检查是否为授权用户（不是匿名施主）
				if wechatUser.Nickname != "匿名施主" {
					// 尝试获取最新的用户信息
					log.Printf("DEBUG: Checking for updated wechat user info")
					userInfo, err := ps.getWechatUserInfo(openid)
					if err == nil {
						// 比较用户信息是否发生变化
						if userInfo["user_name"] != wechatUser.Nickname || userInfo["avatar_url"] != wechatUser.AvatarURL {
							// 用户信息发生变化，更新数据库
							log.Printf("DEBUG: Wechat user info changed, updating database")
							wechatUser.Nickname = userInfo["user_name"]
							wechatUser.AvatarURL = userInfo["avatar_url"]
							if err := utils.DB.Save(&wechatUser).Error; err != nil {
								log.Printf("DEBUG: Failed to update wechat user info: %v", err)
							}
						}
					}
				}
			} else {
				// 没有找到用户信息，使用openid作为user_id
				userID = openid
				log.Printf("DEBUG: No wechat user info found for openid %s, using openid as user_id", openid)
			}
		} else if payment == "alipay" {
			// 支付宝用户，查询支付宝用户表
			var alipayUser models.AlipayUser
			if err := utils.DB.Where("user_id = ?", openid).First(&alipayUser).Error; err == nil {
				// 找到用户信息，使用真实信息
				userID = alipayUser.UserID
				log.Printf("DEBUG: Found alipay user info, using real user_id: %s", userID)
				// 检查是否为授权用户（不是匿名施主）
				if alipayUser.Nickname != "匿名施主" && alipayUser.AccessToken != "" {
					// 尝试获取最新的用户信息
					log.Printf("DEBUG: Checking for updated alipay user info")
					userInfo, err := ps.getAlipayUserInfo(openid)
					if err == nil {
						// 比较用户信息是否发生变化
						if userInfo["user_name"] != alipayUser.Nickname || userInfo["avatar_url"] != alipayUser.AvatarURL {
							// 用户信息发生变化，更新数据库
							log.Printf("DEBUG: Alipay user info changed, updating database")
							alipayUser.Nickname = userInfo["user_name"]
							alipayUser.AvatarURL = userInfo["avatar_url"]
							if err := utils.DB.Save(&alipayUser).Error; err != nil {
								log.Printf("DEBUG: Failed to update alipay user info: %v", err)
							}
						}
					}
				}
			} else {
				// 没有找到用户信息，使用openid作为user_id
				userID = openid
				log.Printf("DEBUG: No alipay user info found for openid %s, using openid as user_id", openid)
			}
		} else {
			// 未知支付类型，使用openid作为user_id
			userID = openid
			log.Printf("DEBUG: Unknown payment type, using openid as user_id: %s", openid)
		}
	}

	// 创建订单
	donation := models.Donation{
		OpenID:          openid, // 保存真实的openid，未授权时为"anonymous"
		Amount:          amount,
		Payment:         payment,
		PaymentConfigID: paymentConfigID, // 保存支付配置ID
		Categories:      categoryID,      // 保存捐款类目ID
		Blessing:        blessing,        // 保存祝福语
		OrderID:         orderID,
		Status:          "pending",
	}

	// 记录openid状态
	if openid == "" {
		log.Printf("DEBUG: Creating order with empty openid (anonymous)")
	} else if openid == "anonymous" {
		log.Printf("DEBUG: Creating order with anonymous openid")
	} else {
		log.Printf("DEBUG: Creating order with real openid: %s", openid)
	}

	if err := utils.DB.Create(&donation).Error; err != nil {
		return "", "", err
	}

	// 启动支付结果轮询（按照文档要求：从跳转5秒后开始轮询）
	go ps.startPaymentPolling(orderID)

	// 返回订单ID和支付URL（WAP支付需要前端跳转到这个URL）
	return orderID, payURL, nil
}

// startPaymentPolling 启动支付结果轮询
// 轮询规范(从跳转5秒后开始轮询):
// - 第0-1分钟，间隔为3秒
// - 第1-5分钟，间隔为10秒
// - 第6分钟，执行最后一次查询
func (ps *PaymentService) startPaymentPolling(orderID string) {
	log.Printf("DEBUG: Starting payment polling for order %s", orderID)

	// 等待5秒后开始轮询（按照文档要求）
	time.Sleep(5 * time.Second)

	startTime := time.Now()
	maxPollingTime := 6 * time.Minute
	isFinalQuery := false

	// 轮询主循环
	for {
		elapsedTime := time.Since(startTime)

		// 计算下一次轮询间隔（提前声明，避免goto跳过变量声明）
		sleepDuration := 3 * time.Second
		if elapsedTime > time.Minute {
			sleepDuration = 10 * time.Second
		}

		// 检查是否超过最大轮询时间
		if elapsedTime > maxPollingTime {
			log.Printf("DEBUG: Max polling time exceeded for order %s, elapsed: %v", orderID, elapsedTime)
			break
		}

		// 执行查询
		log.Printf("DEBUG: Polling order %s, elapsed: %v", orderID, elapsedTime)
		result, err := ps.QueryOrder(orderID)
		if err != nil {
			log.Printf("DEBUG: Polling failed for order %s: %v", orderID, err)
			// 跳转到sleep，此时sleepDuration已经声明
			goto sleep
		}

		// 解析查询结果
		if result != nil {
			// 更新订单状态
			if updated, status := ps.updateOrderStatusFromQuery(orderID, result); updated {
				log.Printf("DEBUG: Order %s status updated to %s via polling", orderID, status)
				// 如果是最终状态，结束轮询
				if status == "completed" || status == "failed" {
					log.Printf("DEBUG: Final status reached for order %s, ending polling", orderID)
					return
				}
			}
		}

		// 第6分钟，执行最后一次查询
		if elapsedTime >= 5*time.Minute && !isFinalQuery {
			isFinalQuery = true
			log.Printf("DEBUG: Final polling attempt for order %s", orderID)
		}

		// 如果是最后一次查询，不需要再等待
		if isFinalQuery {
			break
		}

	sleep:
		// 等待下一次轮询
		time.Sleep(sleepDuration)
	}

	// 最后一次查询前，先检查订单当前状态
	var currentDonation models.Donation
	if err := utils.DB.Where("order_id = ?", orderID).First(&currentDonation).Error; err == nil {
		// 检查当前状态是否已经是最终状态
		if currentDonation.Status == "completed" || currentDonation.Status == "failed" {
			log.Printf("DEBUG: Order %s already has final status %s, skipping final polling", orderID, currentDonation.Status)
			return
		}
	}

	// 最后一次查询
	log.Printf("DEBUG: Final polling check for order %s", orderID)
	result, err := ps.QueryOrder(orderID)
	if err != nil {
		log.Printf("DEBUG: Final polling failed for order %s: %v", orderID, err)
		// 只有当当前状态不是最终状态时，才更新为unknown
		if currentDonation.Status != "completed" && currentDonation.Status != "failed" {
			ps.updateOrderStatus(orderID, "unknown")
		}
		return
	}

	// 解析最终查询结果
	if result != nil {
		// 尝试从结果中获取order_status
		bizResponse, bizOk := result["biz_response"].(map[string]interface{})
		data, dataOk := bizResponse["data"].(map[string]interface{})
		orderStatus, statusOk := data["order_status"].(string)

		// 如果能获取到order_status，根据其值决定最终状态
		if bizOk && dataOk && statusOk {
			var finalStatus string
			switch orderStatus {
			case "PAID":
				finalStatus = "completed" // 支付成功，不要改为unknown
			case "PAY_CANCELED":
				finalStatus = "failed" // 支付失败，不要改为unknown
			default:
				// 只有非最终状态才改为unknown
				if currentDonation.Status != "completed" && currentDonation.Status != "failed" {
					finalStatus = "unknown"
				} else {
					// 如果当前已经是最终状态，保持不变
					log.Printf("DEBUG: Order %s already has final status %s, keeping status", orderID, currentDonation.Status)
					return
				}
			}

			// 更新订单状态
			log.Printf("DEBUG: Final order %s status: %s (order_status: %s)", orderID, finalStatus, orderStatus)
			ps.updateOrderStatus(orderID, finalStatus)
		} else {
			// 如果无法解析order_status，只有当当前状态不是最终状态时，才更新为unknown
			if currentDonation.Status != "completed" && currentDonation.Status != "failed" {
				log.Printf("DEBUG: Final query did not return valid order_status for order %s, updating to unknown", orderID)
				ps.updateOrderStatus(orderID, "unknown")
			} else {
				log.Printf("DEBUG: Order %s already has final status %s, keeping status", orderID, currentDonation.Status)
			}
		}
	} else {
		// 没有结果，只有当当前状态不是最终状态时，才更新为unknown
		if currentDonation.Status != "completed" && currentDonation.Status != "failed" {
			log.Printf("DEBUG: No result from final query for order %s, updating to unknown", orderID)
			ps.updateOrderStatus(orderID, "unknown")
		} else {
			log.Printf("DEBUG: Order %s already has final status %s, keeping status", orderID, currentDonation.Status)
		}
	}
}

// updateOrderStatusFromQuery 根据查询结果更新订单状态
func (ps *PaymentService) updateOrderStatusFromQuery(orderID string, result map[string]interface{}) (bool, string) {
	// 解析查询结果中的状态字段
	// 获取biz_response
	bizResponse, ok := result["biz_response"].(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: Invalid biz_response format for order %s: %v", orderID, result)
		return false, ""
	}

	// 检查biz_response中的result_code
	bizResultCode, _ := bizResponse["result_code"].(string)
	if bizResultCode == "FAIL" {
		// 订单查询失败，检查错误码
		errorCode, _ := bizResponse["error_code"].(string)
		log.Printf("DEBUG: Order query failed for %s - error_code: %s", orderID, errorCode)

		// 如果是订单不存在错误，将订单状态更新为failed
		if errorCode == "UPAY_ORDER_NOT_EXISTS" {
			status := "failed"
			ps.updateOrderStatus(orderID, status)
			return true, status
		}
		return false, ""
	}

	// 获取data
	data, ok := bizResponse["data"].(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: Invalid data format for order %s: %v", orderID, bizResponse)
		return false, ""
	}

	// 获取order_status（第三层级，订单状态码）
	orderStatus, ok := data["order_status"].(string)
	if !ok {
		log.Printf("DEBUG: Missing order_status for order %s: %v", orderID, data)
		return false, ""
	}

	log.Printf("DEBUG: Query result for order %s - order_status: %s", orderID, orderStatus)

	// 根据文档规则映射状态
	var status string
	switch orderStatus {
	case "PAID":
		status = "completed" // 支付成功
	case "PAY_CANCELED":
		status = "failed" // 支付失败
	case "CREATED", "PAY_ERROR":
		status = "pending" // 支付中或状态未知
	default:
		status = "unknown" // 未知状态
	}

	// 更新订单状态
	if status != "pending" || orderStatus == "PAID" || orderStatus == "PAY_CANCELED" {
		ps.updateOrderStatus(orderID, status)

		// 如果支付成功，触发广播（只对微信支付）
		if status == "completed" {
			log.Printf("DEBUG: Payment completed for order %s", orderID)
			// 从订单中获取项目和分类信息
			var donation models.Donation
			if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err == nil {
				// 只对微信支付进行广播
				if donation.Payment == "wechat" {
					// 检查是否已经广播过
					if ps.isBroadcasted(orderID) {
						log.Printf("DEBUG: Order %s already broadcasted, skipping", orderID)
						// 跳过广播逻辑，直接继续执行
					} else {
						// 标记为已广播
						ps.markAsBroadcasted(orderID)
						// 广播逻辑已移除，由 WebSocketManager 直接处理
					}
				}
			}
		}

		return true, status
	}

	return false, ""
}

// updateOrderStatus 更新订单状态到数据库
func (ps *PaymentService) updateOrderStatus(orderID string, status string) {
	// 检查订单是否存在并获取当前状态
	var donation models.Donation
	if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err != nil {
		log.Printf("DEBUG: Failed to find order %s: %v", orderID, err)
		return
	}

	// 只更新状态字段，避免覆盖其他字段
	if donation.Status != status {
		result := utils.DB.Model(&models.Donation{}).Where("order_id = ?", orderID).Update("status", status)
		if result.Error != nil {
			log.Printf("DEBUG: Failed to update status for order %s: %v", orderID, result.Error)
			return
		}

		log.Printf("DEBUG: Successfully updated order %s status from %s to %s", orderID, donation.Status, status)
	}

	// 暂时屏蔽缓存清除功能，因为已经禁用了缓存
	log.Printf("DEBUG: Skipping memory cache clearing for order %s (cache bypassed)", orderID)
}

// HandleCallback 处理支付回调（WAP支付方式）
func (ps *PaymentService) HandleCallback(data map[string]interface{}) error {
	// 添加详细的回调日志
	log.Printf("DEBUG: Handling callback with terminal key - Data: %v", data)

	// 保存原始sign用于验证
	originalSign, _ := data["sign"].(string)
	// 复制map避免修改原始数据
	callbackData := make(map[string]string)
	for k, v := range data {
		// 只处理字符串类型的值用于签名验证
		if strVal, ok := v.(string); ok {
			callbackData[k] = strVal
		}
	}
	// 删除sign字段用于验证
	delete(callbackData, "sign")

	// 验证签名（使用旧的终端密钥验证，兼容旧版调用）
	expectedSign := ps.GenerateSign(callbackData, "terminal")
	if originalSign != expectedSign {
		return fmt.Errorf("invalid sign")
	}

	// 获取订单号（支持多种字段名）
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
		} else if wechatData, ok := data["wechat"].(map[string]interface{}); ok {
			if wxOrderID, ok := wechatData["order_id"].(string); ok {
				orderID = wxOrderID
				log.Printf("Got order ID from wechat.order_id: %s", orderID)
			}
		}
	}
	if orderID == "" {
		return fmt.Errorf("missing order ID")
	}

	// 更新订单状态
	var donation models.Donation
	if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err != nil {
		return err
	}

	// 处理重复回调
	if donation.Status == "completed" {
		return nil // 重复回调，直接返回成功
	}

	// 获取交易状态
	transactionStatus, _ := data["status"].(string)

	// 获取支付方式（从reflect参数中解析）
	paymentType := donation.Payment // 默认为创建订单时的支付方式
	if reflectData, ok := data["reflect"].(string); ok && reflectData != "" {
		var reflectMap map[string]string
		if err := json.Unmarshal([]byte(reflectData), &reflectMap); err == nil {
			if pt, ok := reflectMap["payment"]; ok {
				paymentType = pt
			}
		}
	}

	// 根据交易状态更新订单
	var finalStatus string
	if transactionStatus != "SUCCESS" {
		// 如果支付失败，更新状态为失败
		finalStatus = "failed"
	} else {
		// 支付成功
		finalStatus = "completed"
	}

	// 支付成功，从回调数据中获取用户openid
	var openid string
	if paymentType == "wechat" {
		// 微信openid从payer_uid字段获取
		openid, _ = data["payer_uid"].(string)
	} else {
		// 支付宝user_id从payer_uid字段获取
		openid, _ = data["payer_uid"].(string)
	}

	// 异步获取用户信息，不阻塞回调响应
	if finalStatus == "completed" {
		go func() {
			if paymentType == "wechat" {
				// 使用微信公众号API获取真实用户信息
				ps.getWechatUserInfo(openid)
			} else {
				// 使用支付宝API获取真实用户信息
				ps.getAlipayUserInfo(openid)
			}
		}()
	}

	// 更新捐款记录，记录openid用于关联用户表
	updateData := map[string]interface{}{
		"Status":  finalStatus,
		"Payment": paymentType,
	}

	// 只有当当前订单的OpenID为空时才更新
	if donation.OpenID == "" {
		updateData["OpenID"] = openid // 记录openid，用于关联用户表
	}

	// 存储支付回调中的payer_uid到PayerUID字段
	if openid != "" {
		updateData["PayerUID"] = openid
	}

	// 执行数据库更新
	if err := utils.DB.Model(&donation).Updates(updateData).Error; err != nil {
		return err
	}

	// 调用updateOrderStatus函数清除缓存
	ps.updateOrderStatus(orderID, finalStatus)

	return nil
}

// HandleCallbackWithPublicKey 处理支付回调（使用公钥验证）
func (ps *PaymentService) HandleCallbackWithPublicKey(data map[string]interface{}, authHeader string, rawBody []byte) error {
	// 1. 从Authorization头中提取sign
	sign := authHeader

	// 2. 验证签名
	if !ps.VerifyCallbackSignature(rawBody, sign) {
		return fmt.Errorf("invalid sign")
	}

	// 3. 获取订单号（支持多种字段名）
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
		} else if wechatData, ok := data["wechat"].(map[string]interface{}); ok {
			if wxOrderID, ok := wechatData["order_id"].(string); ok {
				orderID = wxOrderID
				log.Printf("Got order ID from wechat.order_id: %s", orderID)
			}
		}
	}
	if orderID == "" {
		return fmt.Errorf("missing order ID")
	}

	// 4. 更新订单状态
	var donation models.Donation
	if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err != nil {
		return err
	}

	// 5. 处理重复回调
	if donation.Status == "completed" {
		return nil // 重复回调，直接返回成功
	}

	// 6. 获取交易状态
	transactionStatus, _ := data["status"].(string)

	// 8. 获取支付方式（从reflect参数中解析）
	paymentType := donation.Payment // 默认为创建订单时的支付方式
	if reflectData, ok := data["reflect"].(string); ok && reflectData != "" {
		var reflectMap map[string]string
		if err := json.Unmarshal([]byte(reflectData), &reflectMap); err == nil {
			if pt, ok := reflectMap["payment"]; ok {
				paymentType = pt
			}
		}
	}

	// 9. 根据交易状态更新订单
	var finalStatus string
	if transactionStatus == "SUCCESS" {
		// 支付成功
		finalStatus = "completed"
	} else {
		// 支付失败或状态未知
		finalStatus = "failed"
	}

	// 10. 获取用户信息（从回调数据中提取真实用户信息）

	// 从payer_uid字段获取真实的openid或user_id
	openid, _ := data["payer_uid"].(string)

	if finalStatus == "completed" {
		// 异步获取用户信息，不阻塞回调响应
		go func() {
			if paymentType == "wechat" {
				// 使用微信公众号API获取真实用户信息
				ps.getWechatUserInfo(openid)
			} else {
				// 使用支付宝API获取真实用户信息
				ps.getAlipayUserInfo(openid)
			}
		}()
	}

	// 11. 更新捐款记录，记录openid用于关联用户表
	updateData := map[string]interface{}{
		"Status":  finalStatus,
		"Payment": paymentType,
	}

	// 只有当当前订单的OpenID为空时才更新
	if openid != "" && donation.OpenID == "" {
		updateData["OpenID"] = openid
	}

	// 存储支付回调中的payer_uid到PayerUID字段
	if openid != "" {
		updateData["PayerUID"] = openid
	}

	// 执行数据库更新
	if err := utils.DB.Model(&donation).Updates(updateData).Error; err != nil {
		return err
	}

	// 调用updateOrderStatus函数清除缓存
	ps.updateOrderStatus(orderID, finalStatus)

	return nil
}

// VerifyCallbackSignature 使用RSA SHA256WithRSA验证回调签名
func (ps *PaymentService) VerifyCallbackSignature(rawBody []byte, sign string) bool {
	// 收钱吧提供的公钥
	publicKeyPEM := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA5+MNqcjgw4bsSWhJfw2M
+gQB7P+pEiYOfvRmA6kt7Wisp0J3JbOtsLXGnErn5ZY2D8KkSAHtMYbeddphFZQJ
zUbiaDi75GUAG9XS3MfoKAhvNkK15VcCd8hFgNYCZdwEjZrvx6Zu1B7c29S64LQP
HceS0nyXF8DwMIVRcIWKy02cexgX0UmUPE0A2sJFoV19ogAHaBIhx5FkTy+eeBJE
bU03Do97q5G9IN1O3TssvbYBAzugz+yUPww2LadaKexhJGg+5+ufoDd0+V3oFL0/
ebkJvD0uiBzdE3/ci/tANpInHAUDIHoWZCKxhn60f3/3KiR8xuj2vASgEqphxT5O
fwIDAQAB
-----END PUBLIC KEY-----`

	// 解码PEM格式公钥
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		log.Printf("Failed to decode PEM block")
		return false
	}

	// 解析公钥
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Printf("Failed to parse public key: %v", err)
		return false
	}

	// 断言为RSA公钥
	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		log.Printf("Public key is not RSA")
		return false
	}

	// 解码Base64签名
	signBytes, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		log.Printf("Failed to decode sign: %v", err)
		return false
	}

	// 使用SHA256WithRSA验证签名
	hash := sha256.Sum256(rawBody)
	if err := rsa.VerifyPKCS1v15(rsaPubKey, crypto.SHA256, hash[:], signBytes); err != nil {
		log.Printf("Signature verification failed: %v", err)
		return false
	}

	return true
}

// getWechatAccessToken 获取微信公众号access_token（带缓存机制）
func (ps *PaymentService) getWechatAccessToken() (string, error) {
	// 检查微信公众号配置是否完整
	if ps.config.WechatAppID == "" || ps.config.WechatAppSecret == "" {
		return "", fmt.Errorf("wechat appid or appsecret not configured")
	}

	// 检查缓存的access_token是否有效（提前5分钟过期，避免边缘情况）
	now := time.Now()
	if ps.accessToken.AccessToken != "" && ps.accessToken.ExpiresAt.After(now.Add(5*time.Minute)) {
		log.Printf("DEBUG: Using cached wechat access_token")
		return ps.accessToken.AccessToken, nil
	}

	log.Printf("DEBUG: Getting new wechat access_token")

	// 构建请求URL
	accessTokenURL := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		ps.config.WechatAppID, ps.config.WechatAppSecret)

	// 发送请求
	resp, err := ps.httpClient.Get(accessTokenURL)
	if err != nil {
		return "", fmt.Errorf("failed to get access_token: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read access_token response: %v", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode access_token response: %v", err)
	}

	// 检查是否返回了access_token
	accessToken, ok := result["access_token"].(string)
	if !ok {
		return "", fmt.Errorf("access_token not found in response: %s", string(body))
	}

	// 读取过期时间（默认7200秒）
	expiresIn := int64(7200)
	if exp, ok := result["expires_in"].(float64); ok {
		expiresIn = int64(exp)
	}

	// 更新缓存
	ps.accessToken.AccessToken = accessToken
	ps.accessToken.ExpiresAt = now.Add(time.Duration(expiresIn) * time.Second)

	log.Printf("DEBUG: New wechat access_token obtained, expires at: %v", ps.accessToken.ExpiresAt)

	return accessToken, nil
}

// GetWechatAuthURL 生成微信公众号授权URL
func (ps *PaymentService) GetWechatAuthURL(host string) (string, error) {
	// 默认重定向到支付页面
	redirectURL := fmt.Sprintf("http://%s/pay?authorized=1", host)
	return ps.GetWechatAuthURLWithRedirect(host, redirectURL)
}

// GetWechatAuthURLWithRedirect 生成带自定义重定向URL的微信公众号授权URL
func (ps *PaymentService) GetWechatAuthURLWithRedirect(host string, redirectURL string) (string, error) {
	// 检查微信公众号配置是否完整
	if ps.config.WechatAppID == "" {
		return "", fmt.Errorf("wechat appid not configured")
	}

	// 生成回调URL，将重定向URL作为参数传递
	callbackURL := fmt.Sprintf("http://%s/api/wechat/callback?redirect_url=%s", host, url.QueryEscape(redirectURL))

	// 构建授权URL（使用snsapi_userinfo scope获取用户信息）
	authURL := fmt.Sprintf(
		"https://open.weixin.qq.com/connect/oauth2/authorize?appid=%s&redirect_uri=%s&response_type=code&scope=snsapi_userinfo&state=STATE#wechat_redirect",
		ps.config.WechatAppID,
		url.QueryEscape(callbackURL),
	)

	log.Printf("DEBUG: Generated wechat auth URL: %s", authURL)
	return authURL, nil
}

// GetAlipayAuthURL 生成支付宝授权URL
func (ps *PaymentService) GetAlipayAuthURL(host string) (string, error) {
	// 默认重定向到支付页面
	redirectURL := fmt.Sprintf("http://%s/pay?authorized=1", host)
	return ps.GetAlipayAuthURLWithRedirect(host, redirectURL)
}

// GetAlipayAuthURLWithRedirect 生成带自定义重定向URL的支付宝授权URL
func (ps *PaymentService) GetAlipayAuthURLWithRedirect(host string, redirectURL string) (string, error) {
	// 检查支付宝配置是否完整
	if ps.config.AlipayAppID == "" {
		return "", fmt.Errorf("alipay appid not configured")
	}

	// 生成回调URL
	callbackURL := fmt.Sprintf("http://%s/api/alipay/callback", host)

	// 使用state参数传递重定向URL
	state := url.QueryEscape(redirectURL)

	// 构建支付宝授权URL（使用auth_user scope获取用户详细信息）
	authURL := fmt.Sprintf(
		"https://openauth.alipay.com/oauth2/publicAppAuthorize.htm?app_id=%s&scope=auth_user&redirect_uri=%s&state=%s",
		ps.config.AlipayAppID,
		url.QueryEscape(callbackURL),
		state,
	)

	log.Printf("DEBUG: Generated alipay auth URL: %s", authURL)
	return authURL, nil
}

// GetWechatUserInfoByCode 使用授权码获取微信用户信息
func (ps *PaymentService) GetWechatUserInfoByCode(code string) (map[string]interface{}, error) {
	// 检查微信公众号配置是否完整
	if ps.config.WechatAppID == "" || ps.config.WechatAppSecret == "" {
		return nil, fmt.Errorf("wechat appid or appsecret not configured")
	}

	// 1. 使用授权码获取access_token和openid
	accessTokenURL := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/oauth2/access_token?appid=%s&secret=%s&code=%s&grant_type=authorization_code",
		ps.config.WechatAppID,
		ps.config.WechatAppSecret,
		code,
	)

	resp, err := ps.httpClient.Get(accessTokenURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get access_token: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read access_token response: %v", err)
	}

	// 解析响应
	var tokenResult map[string]interface{}
	if err := json.Unmarshal(body, &tokenResult); err != nil {
		return nil, fmt.Errorf("failed to decode access_token response: %v", err)
	}

	// 检查是否返回了错误
	if errCode, ok := tokenResult["errcode"].(float64); ok && errCode != 0 {
		return nil, fmt.Errorf("wechat API returned error: %s", string(body))
	}

	// 提取access_token、openid、refresh_token和过期时间
	authAccessToken, ok := tokenResult["access_token"].(string)
	if !ok {
		return nil, fmt.Errorf("access_token not found in response: %s", string(body))
	}

	openid, ok := tokenResult["openid"].(string)
	if !ok {
		return nil, fmt.Errorf("openid not found in response: %s", string(body))
	}

	// 提取refresh_token
	refreshToken, _ := tokenResult["refresh_token"].(string)

	// 提取过期时间
	expiresIn, _ := tokenResult["expires_in"].(float64)
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	// 2. 使用access_token和openid获取用户信息
	userInfoURL := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/userinfo?access_token=%s&openid=%s&lang=zh_CN",
		authAccessToken,
		openid,
	)

	userResp, err := ps.httpClient.Get(userInfoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %v", err)
	}
	defer userResp.Body.Close()

	userBody, err := ioutil.ReadAll(userResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %v", err)
	}

	// 解析用户信息响应
	var userResult map[string]interface{}
	if err := json.Unmarshal(userBody, &userResult); err != nil {
		return nil, fmt.Errorf("failed to decode user info response: %v", err)
	}

	// 检查是否返回了错误
	if errCode, ok := userResult["errcode"].(float64); ok && errCode != 0 {
		return nil, fmt.Errorf("wechat API returned error: %s", string(userBody))
	}

	// 3. 保存用户信息到数据库
	var wechatUser models.WechatUser
	if err := utils.DB.Where(&models.WechatUser{OpenID: openid}).First(&wechatUser).Error; err != nil {
		// 用户不存在，创建新记录
		wechatUser = models.WechatUser{
			OpenID:       openid,
			Nickname:     userResult["nickname"].(string),
			AvatarURL:    userResult["headimgurl"].(string),
			AccessToken:  authAccessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
		}

		// 可选字段
		if unionID, ok := userResult["unionid"].(string); ok {
			wechatUser.UnionID = unionID
		}

		if gender, ok := userResult["sex"].(float64); ok {
			wechatUser.Gender = int(gender)
		}

		if country, ok := userResult["country"].(string); ok {
			wechatUser.Country = country
		}

		if province, ok := userResult["province"].(string); ok {
			wechatUser.Province = province
		}

		if city, ok := userResult["city"].(string); ok {
			wechatUser.City = city
		}

		if language, ok := userResult["language"].(string); ok {
			wechatUser.Language = language
		}

		if err := utils.DB.Create(&wechatUser).Error; err != nil {
			log.Printf("DEBUG: Failed to save wechat user info to database: %v", err)
		}
	} else {
		// 用户已存在，更新信息
		wechatUser.Nickname = userResult["nickname"].(string)
		wechatUser.AvatarURL = userResult["headimgurl"].(string)
		wechatUser.AccessToken = authAccessToken
		wechatUser.RefreshToken = refreshToken
		wechatUser.ExpiresAt = expiresAt

		if err := utils.DB.Save(&wechatUser).Error; err != nil {
			log.Printf("DEBUG: Failed to update wechat user info: %v", err)
		}
	}

	log.Printf("DEBUG: Successfully obtained wechat user info for openid: %s", openid)
	return userResult, nil
}

// GetAlipayUserInfoByCode 使用授权码获取支付宝用户信息
func (ps *PaymentService) GetAlipayUserInfoByCode(code string) (map[string]string, error) {
	// 检查支付宝配置是否完整
	if ps.config.AlipayAppID == "" || ps.config.AlipayPrivateKey == "" || ps.config.AlipayPublicKey == "" {
		return nil, fmt.Errorf("alipay configuration incomplete")
	}

	// 1. 准备通用请求参数
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	charset := "utf-8"
	// 使用配置的签名类型，默认为RSA2
	signType := ps.config.AlipaySignType
	if signType == "" {
		signType = "RSA2"
	}
	// 使用配置的字符集，默认为utf-8
	if ps.config.AlipayCharset != "" {
		charset = ps.config.AlipayCharset
	}

	// 2. 第一步：使用授权码获取access_token和user_id
	// 构建alipay.system.oauth.token请求参数
	tokenParams := map[string]string{
		"app_id":     ps.config.AlipayAppID,
		"method":     "alipay.system.oauth.token",
		"charset":    charset,
		"sign_type":  signType,
		"timestamp":  timestamp,
		"version":    "1.0",
		"grant_type": "authorization_code",
		"code":       code,
	}

	// 生成签名
	tokenSign := ps.generateAlipaySign(tokenParams)
	if tokenSign == "" {
		return nil, fmt.Errorf("failed to generate sign for token request")
	}
	tokenParams["sign"] = tokenSign

	// 构建请求URL，使用配置的网关地址或默认值
	tokenURL := ps.config.AlipayGatewayURL
	if tokenURL == "" {
		tokenURL = "https://openapi.alipay.com/gateway.do"
	}
	tokenReqBody := ps.buildAlipayRequest(tokenParams)

	// 发送请求
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(tokenReqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	// 设置正确的Content-Type和字符集
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	tokenResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get access_token: %v", err)
	}
	defer tokenResp.Body.Close()

	tokenBody, err := ioutil.ReadAll(tokenResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read access_token response: %v", err)
	}

	// 确保响应体是UTF-8编码
	responseStr := string(tokenBody)
	log.Printf("DEBUG: Raw token response: %s", responseStr)

	// 解析token响应
	var tokenResult map[string]interface{}
	if err := json.Unmarshal([]byte(responseStr), &tokenResult); err != nil {
		return nil, fmt.Errorf("failed to decode access_token response: %v", err)
	}

	// 检查是否返回了错误
	if errorResp, ok := tokenResult["error_response"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("alipay API returned error: %s, %s", errorResp["code"], errorResp["msg"])
	}

	// 提取access_token、user_id、refresh_token和过期时间
	var oauthResp map[string]interface{}
	if resp, ok := tokenResult["alipay_system_oauth_token_response"].(map[string]interface{}); ok {
		oauthResp = resp
	} else {
		return nil, fmt.Errorf("invalid alipay response format: %s", string(tokenBody))
	}

	authAccessToken, _ := oauthResp["access_token"].(string)
	userID, _ := oauthResp["user_id"].(string)
	refreshToken, _ := oauthResp["refresh_token"].(string)

	// 提取过期时间
	expiresIn, _ := oauthResp["expires_in"].(string)
	expiresAt := time.Now()
	if expiresIn != "" {
		if expiresInInt, err := strconv.Atoi(expiresIn); err == nil {
			expiresAt = time.Now().Add(time.Duration(expiresInInt) * time.Second)
		}
	}

	if authAccessToken == "" || userID == "" {
		return nil, fmt.Errorf("access_token or user_id not found in response: %s", string(tokenBody))
	}

	// 3. 第二步：使用access_token获取用户详细信息
	// 构建alipay.user.info.share请求参数
	userInfoParams := map[string]string{
		"app_id":     ps.config.AlipayAppID,
		"method":     "alipay.user.info.share",
		"charset":    charset,
		"sign_type":  signType,
		"timestamp":  timestamp,
		"version":    "1.0",
		"auth_token": authAccessToken,
	}

	// 生成签名
	userInfoSign := ps.generateAlipaySign(userInfoParams)
	if userInfoSign == "" {
		return nil, fmt.Errorf("failed to generate sign for user info request")
	}
	userInfoParams["sign"] = userInfoSign

	// 构建请求URL，使用配置的网关地址或默认值
	userInfoURL := ps.config.AlipayGatewayURL
	if userInfoURL == "" {
		userInfoURL = "https://openapi.alipay.com/gateway.do"
	}
	userInfoReqBody := ps.buildAlipayRequest(userInfoParams)

	// 发送请求
	req, err = http.NewRequest("POST", userInfoURL, strings.NewReader(userInfoReqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	// 设置正确的Content-Type和字符集
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	userInfoResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %v", err)
	}
	defer userInfoResp.Body.Close()

	userInfoBody, err := ioutil.ReadAll(userInfoResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %v", err)
	}

	// 确保响应体是UTF-8编码
	userInfoStr := string(userInfoBody)
	log.Printf("DEBUG: Raw user info response: %s", userInfoStr)

	// 解析user info响应
	var userInfoResult map[string]interface{}
	if err := json.Unmarshal([]byte(userInfoStr), &userInfoResult); err != nil {
		return nil, fmt.Errorf("failed to decode user info response: %v", err)
	}

	// 检查是否返回了错误
	if errorResp, ok := userInfoResult["error_response"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("alipay API returned error: %s, %s", errorResp["code"], errorResp["msg"])
	}

	// 提取用户详细信息
	var userShareResp map[string]interface{}
	if resp, ok := userInfoResult["alipay_user_info_share_response"].(map[string]interface{}); ok {
		userShareResp = resp
	} else {
		return nil, fmt.Errorf("invalid alipay user info response format: %s", string(userInfoBody))
	}

	// 提取用户信息字段
	nickname := ""
	if nick, ok := userShareResp["nick_name"].(string); ok {
		nickname = nick
	}

	avatarURL := ""
	if avatar, ok := userShareResp["avatar"].(string); ok {
		avatarURL = avatar
	}

	// 设置默认值
	if nickname == "" {
		nickname = "支付宝用户"
	}

	if avatarURL == "" {
		avatarURL = "https://via.placeholder.com/100?text=支付宝用户"
	}

	// 3. 保存用户信息到数据库
	var alipayUser models.AlipayUser
	if err := utils.DB.Where("user_id = ?", userID).First(&alipayUser).Error; err != nil {
		// 用户不存在，创建新记录
		alipayUser = models.AlipayUser{
			UserID:       userID,
			Nickname:     nickname,
			AvatarURL:    avatarURL,
			AccessToken:  authAccessToken, // 保存access_token
			RefreshToken: refreshToken,    // 保存refresh_token
			ExpiresAt:    expiresAt,       // 保存过期时间
		}

		if err := utils.DB.Create(&alipayUser).Error; err != nil {
			log.Printf("DEBUG: Failed to save alipay user info to database: %v", err)
		}
	} else {
		// 用户存在，更新信息
		alipayUser.Nickname = nickname
		alipayUser.AvatarURL = avatarURL
		alipayUser.AccessToken = authAccessToken // 更新access_token
		alipayUser.RefreshToken = refreshToken   // 更新refresh_token
		alipayUser.ExpiresAt = expiresAt         // 更新过期时间
		if err := utils.DB.Save(&alipayUser).Error; err != nil {
			log.Printf("DEBUG: Failed to update alipay user info in database: %v", err)
		}
	}

	log.Printf("DEBUG: Successfully obtained alipay user info for user_id: %s, nickname: %s", userID, nickname)

	// 标准化返回结果，与微信保持一致
	return map[string]string{
		"user_id":      alipayUser.UserID,
		"user_name":    alipayUser.Nickname,
		"avatar_url":   alipayUser.AvatarURL,
		"access_token": authAccessToken,
	}, nil
}

// generateAlipaySign 生成支付宝签名
func (ps *PaymentService) generateAlipaySign(params map[string]string) string {
	// 1. 对参数进行排序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 2. 拼接字符串
	var strs []string
	for _, k := range keys {
		if v := params[k]; v != "" {
			strs = append(strs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	strToSign := strings.Join(strs, "&")

	// 3. 处理私钥格式，确保包含正确的PEM标记
	privateKeyStr := ps.config.AlipayPrivateKey

	// 验证私钥完整性
	if privateKeyStr == "" {
		log.Printf("DEBUG: Private key is empty")
		return ""
	}

	// 清理私钥，移除可能的多余字符
	privateKeyStr = strings.TrimSpace(privateKeyStr)
	privateKeyStr = strings.ReplaceAll(privateKeyStr, "\r\n", "\n")
	privateKeyStr = strings.ReplaceAll(privateKeyStr, "\n\n", "\n")

	// 添加缺失的PEM标记（如果没有）
	if !strings.HasPrefix(privateKeyStr, "-----BEGIN") {
		// 对于没有BEGIN标记的私钥，添加正确的PKCS8标记
		privateKeyStr = "-----BEGIN PRIVATE KEY-----\n" + privateKeyStr + "\n-----END PRIVATE KEY-----"
	}

	// 4. 使用私钥进行RSA2签名
	privateKey := []byte(privateKeyStr)
	block, _ := pem.Decode(privateKey)
	if block == nil {
		log.Printf("DEBUG: Failed to decode private key")
		log.Printf("DEBUG: Private key length: %d", len(privateKey))
		log.Printf("DEBUG: Private key prefix: %s", string(privateKey[:100]))
		return ""
	}

	// 尝试不同的私钥解析方式
	var privKey interface{}
	var err error

	// 先尝试PKCS8格式
	privKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// 如果失败，尝试PKCS1格式
		log.Printf("DEBUG: PKCS8 parsing failed, trying PKCS1: %v", err)
		privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			log.Printf("DEBUG: Failed to parse private key: %v", err)
			return ""
		}
	}

	h := sha256.New()
	h.Write([]byte(strToSign))
	sum := h.Sum(nil)

	rsaPrivKey, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		log.Printf("DEBUG: Failed to cast to rsa.PrivateKey")
		return ""
	}

	signature, err := rsa.SignPKCS1v15(nil, rsaPrivKey, crypto.SHA256, sum)
	if err != nil {
		log.Printf("DEBUG: Failed to sign: %v", err)
		return ""
	}

	return base64.StdEncoding.EncodeToString(signature)
}

// buildAlipayRequest 构建支付宝请求体
func (ps *PaymentService) buildAlipayRequest(params map[string]string) string {
	var parts []string
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
	}
	return strings.Join(parts, "&")
}

// refreshWechatToken 使用refresh_token刷新微信access_token
func (ps *PaymentService) refreshWechatToken(refreshToken string) (map[string]interface{}, error) {
	// 检查微信公众号配置是否完整
	if ps.config.WechatAppID == "" || ps.config.WechatAppSecret == "" {
		return nil, fmt.Errorf("wechat appid or appsecret not configured")
	}

	// 构建刷新token的URL
	refreshURL := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/oauth2/refresh_token?appid=%s&grant_type=refresh_token&refresh_token=%s",
		ps.config.WechatAppID,
		refreshToken,
	)

	resp, err := ps.httpClient.Get(refreshURL)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh access_token: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh token response: %v", err)
	}

	// 解析响应
	var tokenResult map[string]interface{}
	if err := json.Unmarshal(body, &tokenResult); err != nil {
		return nil, fmt.Errorf("failed to decode refresh token response: %v", err)
	}

	// 检查是否返回了错误
	if errCode, ok := tokenResult["errcode"].(float64); ok && errCode != 0 {
		return nil, fmt.Errorf("wechat API returned error: %s", string(body))
	}

	return tokenResult, nil
}

// getWechatUserInfo 使用openid获取微信用户信息，只返回已存在的用户信息
func (ps *PaymentService) getWechatUserInfo(openid string) (map[string]string, error) {
	// 先检查数据库中是否已有该用户信息
	var wechatUser models.WechatUser

	// 1. 首先尝试通过openid查找
	if err := utils.DB.Where(&models.WechatUser{OpenID: openid}).First(&wechatUser).Error; err == nil {
		// 数据库中已有用户信息，检查token是否过期
		if time.Now().After(wechatUser.ExpiresAt) && wechatUser.RefreshToken != "" {
			// Token已过期，尝试刷新
			log.Printf("DEBUG: Wechat token expired, refreshing for openid: %s", openid)
			tokenResult, err := ps.refreshWechatToken(wechatUser.RefreshToken)
			if err == nil {
				// 刷新成功，更新数据库中的token信息
				if newAccessToken, ok := tokenResult["access_token"].(string); ok {
					wechatUser.AccessToken = newAccessToken
				}
				if newRefreshToken, ok := tokenResult["refresh_token"].(string); ok {
					wechatUser.RefreshToken = newRefreshToken
				}
				if expiresIn, ok := tokenResult["expires_in"].(float64); ok {
					wechatUser.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
				}
				utils.DB.Save(&wechatUser)
				log.Printf("DEBUG: Wechat token refreshed successfully for openid: %s", openid)
			} else {
				log.Printf("DEBUG: Failed to refresh wechat token: %v", err)
			}
		}

		// 返回用户信息
		log.Printf("DEBUG: Wechat user info found in database for openid: %s", openid)
		return map[string]string{
			"user_id":    wechatUser.OpenID,
			"user_name":  wechatUser.Nickname,
			"avatar_url": wechatUser.AvatarURL,
		}, nil
	}

	// 数据库中没有用户信息，返回空信息
	log.Printf("DEBUG: Wechat user info not found in database for openid: %s", openid)
	return map[string]string{
		"user_id":    openid,
		"user_name":  "",
		"avatar_url": "",
	}, fmt.Errorf("user not found")
}

// refreshAlipayToken 使用refresh_token刷新支付宝access_token
func (ps *PaymentService) refreshAlipayToken(refreshToken string) (map[string]interface{}, error) {
	// 检查支付宝配置是否完整
	if ps.config.AlipayAppID == "" || ps.config.AlipayPrivateKey == "" || ps.config.AlipayPublicKey == "" {
		return nil, fmt.Errorf("alipay configuration incomplete")
	}

	// 1. 准备通用请求参数
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	charset := "utf-8"
	// 使用配置的签名类型，默认为RSA2
	signType := ps.config.AlipaySignType
	if signType == "" {
		signType = "RSA2"
	}
	// 使用配置的字符集，默认为utf-8
	if ps.config.AlipayCharset != "" {
		charset = ps.config.AlipayCharset
	}

	// 2. 构建alipay.system.oauth.token请求参数（使用refresh_token）
	tokenParams := map[string]string{
		"app_id":        ps.config.AlipayAppID,
		"method":        "alipay.system.oauth.token",
		"charset":       charset,
		"sign_type":     signType,
		"timestamp":     timestamp,
		"version":       "1.0",
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}

	// 生成签名
	tokenSign := ps.generateAlipaySign(tokenParams)
	if tokenSign == "" {
		return nil, fmt.Errorf("failed to generate sign for token request")
	}
	tokenParams["sign"] = tokenSign

	// 构建请求URL，使用配置的网关地址或默认值
	tokenURL := ps.config.AlipayGatewayURL
	if tokenURL == "" {
		tokenURL = "https://openapi.alipay.com/gateway.do"
	}
	tokenReqBody := ps.buildAlipayRequest(tokenParams)

	// 发送请求
	tokenResp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(tokenReqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to refresh access_token: %v", err)
	}
	defer tokenResp.Body.Close()

	tokenBody, err := ioutil.ReadAll(tokenResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh token response: %v", err)
	}

	// 解析token响应
	var tokenResult map[string]interface{}
	if err := json.Unmarshal(tokenBody, &tokenResult); err != nil {
		return nil, fmt.Errorf("failed to decode refresh token response: %v", err)
	}

	// 检查是否返回了错误
	if errorResp, ok := tokenResult["error_response"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("alipay API returned error: %s, %s", errorResp["code"], errorResp["msg"])
	}

	// 提取响应数据
	var oauthResp map[string]interface{}
	if resp, ok := tokenResult["alipay_system_oauth_token_response"].(map[string]interface{}); ok {
		oauthResp = resp
	} else {
		return nil, fmt.Errorf("invalid alipay response format: %s", string(tokenBody))
	}

	return oauthResp, nil
}

// getAlipayUserInfo 使用user_id获取支付宝用户信息，只返回已存在的用户信息
func (ps *PaymentService) getAlipayUserInfo(userID string) (map[string]string, error) {
	// 先检查数据库中是否已有该用户信息
	var alipayUser models.AlipayUser
	if err := utils.DB.Where("user_id = ?", userID).First(&alipayUser).Error; err != nil {
		// 用户不存在，返回空信息
		log.Printf("DEBUG: Alipay user info not found in database for user_id: %s", userID)
		return map[string]string{
			"user_id":    userID,
			"user_name":  "",
			"avatar_url": "",
		}, fmt.Errorf("user not found")
	} else {
		log.Printf("DEBUG: Alipay user info found in database for user_id: %s", userID)
		log.Printf("DEBUG: User has access_token: %t", alipayUser.AccessToken != "")
		log.Printf("DEBUG: Current nickname: %s", alipayUser.Nickname)
		log.Printf("DEBUG: Current avatar: %s", alipayUser.AvatarURL)
	}

	// 检查token是否过期
	if time.Now().After(alipayUser.ExpiresAt) && alipayUser.RefreshToken != "" {
		// Token已过期，尝试刷新
		log.Printf("DEBUG: Alipay token expired, refreshing for user_id: %s", userID)
		tokenResult, err := ps.refreshAlipayToken(alipayUser.RefreshToken)
		if err == nil {
			// 刷新成功，更新数据库中的token信息
			if newAccessToken, ok := tokenResult["access_token"].(string); ok {
				alipayUser.AccessToken = newAccessToken
			}
			if newRefreshToken, ok := tokenResult["refresh_token"].(string); ok {
				alipayUser.RefreshToken = newRefreshToken
			}
			if expiresIn, ok := tokenResult["expires_in"].(string); ok {
				if expiresInInt, err := strconv.Atoi(expiresIn); err == nil {
					alipayUser.ExpiresAt = time.Now().Add(time.Duration(expiresInInt) * time.Second)
				}
			}
			utils.DB.Save(&alipayUser)
			log.Printf("DEBUG: Alipay token refreshed successfully for user_id: %s", userID)
		} else {
			log.Printf("DEBUG: Failed to refresh alipay token: %v", err)
		}
	}

	// 检查是否有access_token，如果有则调用alipay.user.info.share获取真实用户信息
	if alipayUser.AccessToken != "" {
		log.Printf("DEBUG: Using access_token to get real user info for user_id: %s", userID)

		// 1. 准备通用请求参数
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		charset := "utf-8"
		// 使用配置的签名类型，默认为RSA2
		signType := ps.config.AlipaySignType
		if signType == "" {
			signType = "RSA2"
		}
		// 使用配置的字符集，默认为utf-8
		if ps.config.AlipayCharset != "" {
			charset = ps.config.AlipayCharset
		}

		// 2. 构建alipay.user.info.share请求参数
		userInfoParams := map[string]string{
			"app_id":     ps.config.AlipayAppID,
			"method":     "alipay.user.info.share",
			"charset":    charset,
			"sign_type":  signType,
			"timestamp":  timestamp,
			"version":    "1.0",
			"auth_token": alipayUser.AccessToken,
		}

		// 3. 生成签名
		userInfoSign := ps.generateAlipaySign(userInfoParams)
		userInfoParams["sign"] = userInfoSign

		// 4. 构建请求URL，使用配置的网关地址或默认值
		userInfoURL := ps.config.AlipayGatewayURL
		if userInfoURL == "" {
			userInfoURL = "https://openapi.alipay.com/gateway.do"
		}
		userInfoReqBody := ps.buildAlipayRequest(userInfoParams)

		// 5. 发送请求
		log.Printf("DEBUG: Sending request to alipay.user.info.share API for user_id: %s", userID)
		req, err := http.NewRequest("POST", userInfoURL, strings.NewReader(userInfoReqBody))
		if err != nil {
			log.Printf("DEBUG: Failed to create request: %v", err)
		} else {
			// 设置正确的Content-Type和字符集
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
			req.Header.Set("Accept", "application/json; charset=utf-8")
			userInfoResp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("DEBUG: Failed to get user info from alipay API: %v", err)
			} else {
				defer userInfoResp.Body.Close()

				// 6. 读取响应
				userInfoBody, err := ioutil.ReadAll(userInfoResp.Body)
				if err != nil {
					log.Printf("DEBUG: Failed to read user info response: %v", err)
				} else {
					log.Printf("DEBUG: Received response from alipay.user.info.share API: %s", string(userInfoBody))

					// 7. 解析user info响应
					var userInfoResult map[string]interface{}
					if err := json.Unmarshal(userInfoBody, &userInfoResult); err != nil {
						log.Printf("DEBUG: Failed to decode user info response: %v", err)
						log.Printf("DEBUG: Response body: %s", string(userInfoBody))
					} else {
						// 8. 检查是否返回了错误
						if errorResp, ok := userInfoResult["error_response"].(map[string]interface{}); ok {
							log.Printf("DEBUG: Alipay API returned error: %s, %s", errorResp["code"], errorResp["msg"])
						} else {
							// 9. 提取用户详细信息
							var userShareResp map[string]interface{}
							if resp, ok := userInfoResult["alipay_user_info_share_response"].(map[string]interface{}); ok {
								userShareResp = resp

								// 10. 提取用户信息字段
								if nick, ok := userShareResp["nick_name"].(string); ok && nick != "" {
									log.Printf("DEBUG: Found nickname: %s", nick)
									alipayUser.Nickname = nick
								}

								if avatar, ok := userShareResp["avatar"].(string); ok && avatar != "" {
									log.Printf("DEBUG: Found avatar: %s", avatar)
									alipayUser.AvatarURL = avatar
								}

								// 提取其他可选字段
								if gender, ok := userShareResp["gender"].(string); ok {
									alipayUser.Gender = gender
									log.Printf("DEBUG: Found gender: %s", gender)
								}

								if province, ok := userShareResp["province"].(string); ok {
									alipayUser.Province = province
									log.Printf("DEBUG: Found province: %s", province)
								}

								if city, ok := userShareResp["city"].(string); ok {
									alipayUser.City = city
									log.Printf("DEBUG: Found city: %s", city)
								}

								// 11. 更新数据库中的用户信息
								if err := utils.DB.Save(&alipayUser).Error; err != nil {
									log.Printf("DEBUG: Failed to update alipay user info: %v", err)
								} else {
									log.Printf("DEBUG: Updated alipay user info with real data for user_id: %s, nickname: %s", userID, alipayUser.Nickname)
								}
							} else {
								log.Printf("DEBUG: Invalid alipay user info response format: %s", string(userInfoBody))
							}
						}
					}
				}
			}
		}
	} else {
		log.Printf("DEBUG: No access_token found for user_id: %s, cannot get real user info", userID)

	}

	// 构建返回结果
	return map[string]string{
		"user_id":    alipayUser.UserID,
		"user_name":  alipayUser.Nickname,
		"avatar_url": alipayUser.AvatarURL,
	}, nil
}

// RankingItem 排行榜项，包含用户信息
type RankingItem struct {
	ID              uint      `json:"id"`
	OpenID          string    `json:"openid"`
	UserID          string    `json:"user_id"`
	UserName        string    `json:"user_name"`
	AvatarURL       string    `json:"avatar_url"`
	Amount          float64   `json:"amount"`
	Payment         string    `json:"payment"`
	OrderID         string    `json:"order_id"`
	Status          string    `json:"status"`
	PaymentConfigID string    `json:"payment_config_id"`
	CategoryID      string    `json:"category_id"`
	Categories      string    `json:"categories"`
	CategoryName    string    `json:"category_name"`
	Blessing        string    `json:"blessing"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// GetRankings 获取捐款排行榜
func (ps *PaymentService) GetRankings(limit int, offset int, paymentConfigID string, categoryID string) ([]RankingItem, error) {
	var donations []models.Donation

	// 构建查询
	query := utils.DB.Where("status = ?", "completed")

	// 根据paymentConfigID过滤
	if paymentConfigID != "" {
		query = query.Where("payment_config_id = ?", paymentConfigID)
	}

	// 根据categoryID过滤
	if categoryID != "" {
		query = query.Where("categories = ?", categoryID)
	}

	// 执行查询，按创建时间倒序排序，实现真正的分页
	if err := query.Order("created_at desc").Limit(limit).Offset(offset).Find(&donations).Error; err != nil {
		return nil, err
	}

	// 关联查询用户信息，构建排行榜项
	rankings := make([]RankingItem, len(donations))
	var wg sync.WaitGroup
	var mutex sync.Mutex

	// 并发查询每个捐款记录的相关信息
	for i, donation := range donations {
		wg.Add(1)
		go func(index int, don models.Donation) {
			defer wg.Done()

			// 初始化排行榜项
			rankingItem := RankingItem{
				ID:              don.ID,
				OpenID:          don.OpenID,
				UserID:          "",
				Amount:          don.Amount,
				Payment:         don.Payment,
				OrderID:         don.OrderID,
				Status:          don.Status,
				PaymentConfigID: don.PaymentConfigID,
				CategoryID:      don.Categories,
				Categories:      don.Categories,
				CategoryName:    "",
				Blessing:        don.Blessing,
				CreatedAt:       don.CreatedAt,
				UpdatedAt:       don.UpdatedAt,
				UserName:        "",
				AvatarURL:       "",
			}

			// 查询类目名称
			if don.Categories != "" {
				var category models.Category
				if err := utils.DB.Where("id = ?", don.Categories).First(&category).Error; err == nil {
					rankingItem.CategoryName = category.Name
				}
			}

			// 根据支付类型关联不同的用户表获取用户信息
			if don.Payment == "wechat" && don.OpenID != "" && don.OpenID != "anonymous" {
				// 微信用户，关联WechatUser表，但跳过anonymous用户
				var wechatUser models.WechatUser
				if err := utils.DB.Where(&models.WechatUser{OpenID: don.OpenID}).First(&wechatUser).Error; err == nil {
					rankingItem.UserID = wechatUser.OpenID
					rankingItem.UserName = wechatUser.Nickname
					rankingItem.AvatarURL = wechatUser.AvatarURL
				}
			} else if don.Payment == "alipay" && don.OpenID != "" && don.OpenID != "anonymous" {
				// 支付宝用户，关联AlipayUser表，但跳过anonymous用户
				var alipayUser models.AlipayUser
				if err := utils.DB.Where("user_id = ?", don.OpenID).First(&alipayUser).Error; err == nil {
					rankingItem.UserID = alipayUser.UserID
					rankingItem.UserName = alipayUser.Nickname
					rankingItem.AvatarURL = alipayUser.AvatarURL
				}
			}

			// 如果没有找到用户信息，设置默认值
			if rankingItem.UserName == "" {
				rankingItem.UserName = "匿名施主"
			}
			if rankingItem.AvatarURL == "" {
				rankingItem.AvatarURL = "./static/avatar.jpeg"
			}

			// 加锁更新排行榜项
			mutex.Lock()
			rankings[index] = rankingItem
			mutex.Unlock()
		}(i, donation)
	}

	// 等待所有并发查询完成
	wg.Wait()

	return rankings, nil
}

// GetLatestDonation 获取最新的捐款记录
func (ps *PaymentService) GetLatestDonation() (*RankingItem, error) {
	var donation models.Donation

	// 查询最新的已完成捐款记录
	if err := utils.DB.Where("status = ?", "completed").Order("created_at desc").First(&donation).Error; err != nil {
		return nil, err
	}

	// 构建排行榜项
	rankingItem := &RankingItem{
		ID:              donation.ID,
		OpenID:          donation.OpenID,
		UserID:          "",
		Amount:          donation.Amount,
		Payment:         donation.Payment,
		OrderID:         donation.OrderID,
		Status:          donation.Status,
		PaymentConfigID: donation.PaymentConfigID,
		CategoryID:      donation.Categories,
		Categories:      donation.Categories,
		CategoryName:    "",
		Blessing:        donation.Blessing,
		CreatedAt:       donation.CreatedAt,
		UpdatedAt:       donation.UpdatedAt,
		UserName:        "",
		AvatarURL:       "",
	}

	// 查询类目名称
	if donation.Categories != "" {
		var category models.Category
		if err := utils.DB.Where("id = ?", donation.Categories).First(&category).Error; err == nil {
			rankingItem.CategoryName = category.Name
		}
	}

	// 根据支付类型关联不同的用户表获取用户信息
	if donation.Payment == "wechat" && donation.OpenID != "" && donation.OpenID != "anonymous" {
		// 微信用户，关联WechatUser表，但跳过anonymous用户
		var wechatUser models.WechatUser
		if err := utils.DB.Where(&models.WechatUser{OpenID: donation.OpenID}).First(&wechatUser).Error; err == nil {
			rankingItem.UserID = wechatUser.OpenID
			rankingItem.UserName = wechatUser.Nickname
			rankingItem.AvatarURL = wechatUser.AvatarURL
		}
	} else if donation.Payment == "alipay" && donation.OpenID != "" && donation.OpenID != "anonymous" {
		// 支付宝用户，关联AlipayUser表，但跳过anonymous用户
		var alipayUser models.AlipayUser
		if err := utils.DB.Where("user_id = ?", donation.OpenID).First(&alipayUser).Error; err == nil {
			rankingItem.UserID = alipayUser.UserID
			rankingItem.UserName = alipayUser.Nickname
			rankingItem.AvatarURL = alipayUser.AvatarURL
		}
	}

	// 如果没有找到用户信息，设置默认值
	if rankingItem.UserName == "" {
		rankingItem.UserName = "匿名施主"
	}
	if rankingItem.AvatarURL == "" {
		rankingItem.AvatarURL = "./static/avatar.jpeg"
	}

	return rankingItem, nil
}

// GetDonationByOrderID 根据订单ID获取捐款记录
func (ps *PaymentService) GetDonationByOrderID(orderID string) (*RankingItem, error) {
	var donation models.Donation

	// 根据订单ID查询捐款记录
	if err := utils.DB.Where("order_id = ?", orderID).First(&donation).Error; err != nil {
		return nil, err
	}

	// 构建排行榜项
	rankingItem := &RankingItem{
		ID:              donation.ID,
		OpenID:          donation.OpenID,
		UserID:          "",
		Amount:          donation.Amount,
		Payment:         donation.Payment,
		OrderID:         donation.OrderID,
		Status:          donation.Status,
		PaymentConfigID: donation.PaymentConfigID,
		CategoryID:      donation.Categories,
		Categories:      donation.Categories,
		CategoryName:    "",
		Blessing:        donation.Blessing,
		CreatedAt:       donation.CreatedAt,
		UpdatedAt:       donation.UpdatedAt,
		UserName:        "",
		AvatarURL:       "",
	}

	// 查询类目名称
	if donation.Categories != "" {
		var category models.Category
		if err := utils.DB.Where("id = ?", donation.Categories).First(&category).Error; err == nil {
			rankingItem.CategoryName = category.Name
		}
	}

	// 根据支付类型关联不同的用户表获取用户信息
	if donation.Payment == "wechat" && donation.OpenID != "" && donation.OpenID != "anonymous" {
		// 微信用户，关联WechatUser表，但跳过anonymous用户
		var wechatUser models.WechatUser
		if err := utils.DB.Where(&models.WechatUser{OpenID: donation.OpenID}).First(&wechatUser).Error; err == nil {
			rankingItem.UserID = wechatUser.OpenID
			rankingItem.UserName = wechatUser.Nickname
			rankingItem.AvatarURL = wechatUser.AvatarURL
		}
	} else if donation.Payment == "alipay" && donation.OpenID != "" && donation.OpenID != "anonymous" {
		// 支付宝用户，关联AlipayUser表，但跳过anonymous用户
		var alipayUser models.AlipayUser
		if err := utils.DB.Where(&models.AlipayUser{UserID: donation.OpenID}).First(&alipayUser).Error; err == nil {
			rankingItem.UserID = alipayUser.UserID
			rankingItem.UserName = alipayUser.Nickname
			rankingItem.AvatarURL = alipayUser.AvatarURL
		}
	}

	return rankingItem, nil
}
