package router

import (
	"github.com/gin-gonic/gin"
)

type MiddlewareFunc func(internalFunc gin.HandlerFunc) gin.HandlerFunc

type MiddlewareChain struct {
	middlewares []MiddlewareFunc
}

func NewMiddlewareChain(middlewares ...MiddlewareFunc) *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: middlewares,
	}
}

func (mc *MiddlewareChain) Add(middleware MiddlewareFunc) *MiddlewareChain {
	mc.middlewares = append(mc.middlewares, middleware)
	return mc
}

func (mc *MiddlewareChain) Execute(handler gin.HandlerFunc) gin.HandlerFunc {
	result := handler
	for i := len(mc.middlewares) - 1; i >= 0; i-- {
		result = mc.middlewares[i](result)
	}
	return result
}
