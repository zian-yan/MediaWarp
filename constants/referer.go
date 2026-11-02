package constants

// ReferrerPolicy 定义 HTTP Referer 策略类型
type ReferrerPolicy string

const (
	SameOrigin ReferrerPolicy = "same-origin" // 仅同域发送
)
