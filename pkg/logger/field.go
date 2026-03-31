package logger

import (
	"fmt"
	"time"
)

// Field 结构化日志字段
type Field struct {
	Key   string
	Value interface{}
}

// String 创建字符串字段
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Int 创建整数字段
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Int64 创建int64字段
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Duration 创建时间间隔字段
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value.Milliseconds()}
}

// Err 创建错误字段
func Err(err error) Field {
	return Field{Key: "error", Value: err.Error()}
}

// Error 创建错误字段(带自定义key)
func Error(key string, err error) Field {
	return Field{Key: key, Value: err.Error()}
}

// Bool 创建布尔字段
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

// Any 创建任意类型字段
func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// formatValue 格式化字段值
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		// 包含空格或特殊字符时加引号
		if needsQuote(val) {
			return fmt.Sprintf("%q", val)
		}
		return val
	case int, int64, int32, int16, int8:
		return fmt.Sprintf("%d", val)
	case uint, uint64, uint32, uint16, uint8:
		return fmt.Sprintf("%d", val)
	case float64, float32:
		return fmt.Sprintf("%.2f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case time.Duration:
		return fmt.Sprintf("%dms", val.Milliseconds())
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// needsQuote 检查字符串是否需要引号
func needsQuote(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '=' || c == '"' || c == '\n' || c == '\t' {
			return true
		}
	}
	return false
}
