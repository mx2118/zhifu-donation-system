package models

import (
	"time"
)

// Category 捐款类目表
type Category struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"size:50" json:"name"`              // 类目名称，例如：菜蔬
	PaymentConfigID string    `gorm:"size:20;index" json:"payment_config_id"` // 支付配置ID
	Payment         string    `gorm:"size:20;index" json:"payment"`           // 支付参数，用于区分不同配置
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
