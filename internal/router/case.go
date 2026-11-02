package router

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func QueryKeyCaseInsensitive(internalFunc gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		queryParams := make(url.Values)
		for key, values := range ctx.Request.URL.Query() {
			// 只保留首个值：用 , 合并会改变上游对重复参数的解释
			if len(values) > 0 {
				queryParams.Set(strings.ToLower(key), values[0])
			}
		}
		ctx.Request.URL.RawQuery = queryParams.Encode()

		internalFunc(ctx)
	}
}

var _ MiddlewareFunc = QueryKeyCaseInsensitive
