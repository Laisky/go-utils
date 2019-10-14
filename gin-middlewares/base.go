package middlewares

import (
	"context"
	"net/http"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/gin-gonic/gin"
)

// GinCtxKey key of gin ctx that saved in request.context
var GinCtxKey utils.CtxKeyT

// FromStd convert std handler to gin.Handler, with gin context embedded
func FromStd(handler http.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		r2 := ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), GinCtxKey, ctx))
		handler(ctx.Writer, r2)
	}
}

// GetGinCtxFromStdCtx get gin context from standard request.context by GinCtxKey
func GetGinCtxFromStdCtx(ctx context.Context) *gin.Context {
	return ctx.Value(GinCtxKey).(*gin.Context)
}

// LoggerMiddleware middleware to logging
func LoggerMiddleware(ctx *gin.Context) {
	utils.Logger.Debug("request",
		zap.String("path", ctx.Request.RequestURI),
		zap.String("method", ctx.Request.Method),
	)
	ctx.Next()
}
