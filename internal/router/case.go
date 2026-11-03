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
			if len(values) > 0 {
				queryParams.Set(strings.ToLower(key), values[0])
			}
		}
		ctx.Request.URL.RawQuery = queryParams.Encode()

		internalFunc(ctx)
	}
}

var _ MiddlewareFunc = QueryKeyCaseInsensitive
