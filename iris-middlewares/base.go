package irisMiddlewares

import (
	"context"
	"net/http"

	"github.com/kataras/iris"
)

type key string

// IrisCtxKey key of iris ctx that saved in request.context
const IrisCtxKey key = "irisctx"

// FromStd convert std handler to iris.Handler, with iris context embedded
func FromStd(handler http.HandlerFunc) iris.Handler {
	return func(ctx iris.Context) {
		r2 := ctx.Request().WithContext(context.WithValue(ctx.Request().Context(), IrisCtxKey, ctx))
		handler(ctx.ResponseWriter(), r2)
	}
}

// GetIrisCtxFromStdCtx get iris context from standard request.context by IrisCtxKey
func GetIrisCtxFromStdCtx(ctx context.Context) iris.Context {
	return ctx.Value(IrisCtxKey).(iris.Context)
}
