package models

import (
	"time"
)

// WechatUser 微信用户信息表
type WechatUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	OpenID       string    `gorm:"column:open_id;size:50;uniqueIndex" json:"openid"`
	UnionID      string    `gorm:"column:union_id;size:50" json:"unionid"`
	Nickname     string    `gorm:"size:100" json:"nickname"`
	AvatarURL    string    `gorm:"size:255" json:"avatar_url"`
	Gender       int       `json:"gender"` // 0:未知, 1:男, 2:女
	Country      string    `gorm:"size:50" json:"country"`
	Province     string    `gorm:"size:50" json:"province"`
	City         string    `gorm:"size:50" json:"city"`
	Language     string    `gorm:"size:20" json:"language"`
	AccessToken  string    `gorm:"size:255" json:"access_token"`
	RefreshToken string    `gorm:"size:255" json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AlipayUser 支付宝用户信息表
type AlipayUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       string    `gorm:"size:50;uniqueIndex" json:"user_id"` // 支付宝用户唯一标识
	Nickname     string    `gorm:"size:100" json:"nickname"`
	AvatarURL    string    `gorm:"size:255" json:"avatar_url"`
	Gender       string    `gorm:"size:10" json:"gender"` // F:女, M:男, UNKNOWN:未知
	Province     string    `gorm:"size:50" json:"province"`
	City         string    `gorm:"size:50" json:"city"`
	AccessToken  string    `gorm:"size:255" json:"access_token"`  // 支付宝access_token
	RefreshToken string    `gorm:"size:255" json:"refresh_token"` // 支付宝refresh_token
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
