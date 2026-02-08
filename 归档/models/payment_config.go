package models

import (
	"time"
)

// PaymentConfig 合并后的支付配置表模型
type PaymentConfig struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	// 开发者配置
	VendorSN     string    `gorm:"size:50;uniqueIndex" json:"vendor_sn"`
	VendorKey    string    `gorm:"size:100" json:"vendor_key"`
	AppID        string    `gorm:"size:50" json:"app_id"`
	
	// 终端配置
	TerminalSN   string    `gorm:"size:50;uniqueIndex" json:"terminal_sn"`
	TerminalKey  string    `gorm:"size:100" json:"terminal_key"`
	MerchantSN   string    `gorm:"size:50" json:"merchant_sn"`
	MerchantName string    `gorm:"size:255" json:"merchant_name"`
	StoreSN      string    `gorm:"size:50" json:"store_sn"`
	StoreName    string    `gorm:"size:255" json:"store_name"`
	
	// 设备配置
	DeviceID     string    `gorm:"size:50" json:"device_id"`
	
	// API配置
	APIURL       string    `gorm:"size:255" json:"api_url"`
	GatewayURL   string    `gorm:"size:255" json:"gateway_url"`
	
	// 业务配置
	MerchantID   string    `gorm:"size:50" json:"merchant_id"`
	StoreID      string    `gorm:"size:50" json:"store_id"`
	// 品牌配置
	LogoURL      string    `gorm:"size:255" json:"logo_url"`
	Title2       string    `gorm:"size:255" json:"title2"`
	Title3       string    `gorm:"size:255" json:"title3"`
	
	// 微信公众号配置
	WechatAppID     string    `gorm:"size:50" json:"wechat_app_id"`
	WechatAppSecret string    `gorm:"size:100" json:"wechat_app_secret"`
	WechatToken     string    `gorm:"size:100" json:"wechat_token"`
	WechatAESKey    string    `gorm:"size:100" json:"wechat_aes_key"`
	
	// 支付宝配置
	AlipayAppID       string    `gorm:"size:50" json:"alipay_app_id"`
	AlipayPublicKey   string    `gorm:"size:500" json:"alipay_public_key"`   // 支付宝公钥
	AlipayPrivateKey  string    `gorm:"size:500" json:"alipay_private_key"`  // 应用私钥
	
	// 管理字段
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	LastSignInAt time.Time `json:"last_sign_in_at"`
	Description  string    `gorm:"size:255" json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}