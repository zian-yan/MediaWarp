package middleware

import (
	"net/http"
	"strings"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"

	"github.com/gin-gonic/gin"
)

func ClientFilter() gin.HandlerFunc {
	enabled := config.ClientFilter.Enable
	mode := config.ClientFilter.Mode
	clientList := config.ClientFilter.ClientList

	return func(ctx *gin.Context) {
		if !enabled {
			ctx.Next()
			return
		}

		userAgent := ctx.Request.UserAgent()
		var allowed bool
		if userAgent == "" {
			allowed = false
		} else {
			switch mode {
			case constants.WHITELIST:
				allowed = false
				for _, ua := range clientList {
					if strings.Contains(userAgent, ua) {
						allowed = true
						break
					}
				}
			case constants.BLACKLIST:
				allowed = true
				for _, ua := range clientList {
					if strings.Contains(userAgent, ua) {
						allowed = false
						break
					}
				}
			}
		}
		if !allowed {
			ctx.AbortWithStatus(http.StatusForbidden)
			logging.Info("客户端过滤器拦截了请求，User-Agent: ", userAgent)
			return
		}
		logging.Debug("客户端过滤器放行了请求，User-Agent: ", userAgent)
		ctx.Next()

	}
}
