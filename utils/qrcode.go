package utils

import (
	"github.com/skip2/go-qrcode"
)

// GenerateQRCode 生成二维码
func GenerateQRCode(text string) ([]byte, error) {
	// 使用 skip2/go-qrcode 库生成二维码
	return qrcode.Encode(text, qrcode.Medium, 256)
}
