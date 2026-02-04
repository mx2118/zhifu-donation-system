package utils

import (
	"github.com/skip2/go-qrcode"
)

// GenerateQRCode 生成二维码
func GenerateQRCode(text string) ([]byte, error) {
	// 使用skip2/go-qrcode库生成PNG格式的二维码
	return qrcode.Encode(text, qrcode.Medium, 256)
}
