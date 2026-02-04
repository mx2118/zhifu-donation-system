package models

import (
	"time"
)

type Donation struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	OpenID          string    `gorm:"size:50" json:"openid"` // 微信openid或支付宝user_id
	PayerUID        string    `gorm:"size:50" json:"payer_uid"` // 支付回调中的payer_uid
	Amount          float64   `gorm:"type:decimal(10,2)" json:"amount"`
	Payment         string    `gorm:"size:20" json:"payment"`           // wechat, alipay
	PaymentConfigID string    `gorm:"size:20" json:"payment_config_id"` // 支付配置ID
	Categories      string    `gorm:"size:20" json:"categories"`        // 捐款类目
	Blessing        string    `gorm:"size:200" json:"blessing"`         // 祝福语
	OrderID         string    `gorm:"size:50" json:"order_id"`
	Status          string    `gorm:"size:20" json:"status"` // pending, completed
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
