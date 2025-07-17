package utils

import (
	"math/rand"
)

// 生成指定长度的随机字符串，字母数字混合
func RandomString() string {
	// 生成0~64之间的随机长度
	n := rand.Intn(64-16) + 16
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
