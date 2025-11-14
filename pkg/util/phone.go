package util

import (
	"regexp"
)

var (
	// 中国手机号正则表达式
	phoneRegex = regexp.MustCompile(`^1[3-9]\d{9}$`)
)

// ValidatePhone 验证手机号格式（中国手机号）
func ValidatePhone(phone string) bool {
	if phone == "" {
		return false
	}
	return phoneRegex.MatchString(phone)
}

// NormalizePhone 规范化手机号（去除空格、横线等）
func NormalizePhone(phone string) string {
	// 去除所有非数字字符
	re := regexp.MustCompile(`\D`)
	return re.ReplaceAllString(phone, "")
}

