package handler

import (
	"regexp"

	"github.com/gin-gonic/gin"
)

type RegexpRouteRule struct {
	Regexp  *regexp.Regexp
	Handler gin.HandlerFunc
}
