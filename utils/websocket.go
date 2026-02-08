package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateConnID 生成连接ID
func GenerateConnID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 如果随机数生成失败，使用时间戳+随机数
		return fmt.Sprintf("%d%x", time.Now().UnixNano(), b)
	}
	return hex.EncodeToString(b)
}

// Now 获取当前时间字符串
func Now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
