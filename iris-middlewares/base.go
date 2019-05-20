package irisMiddlewares

import (
	"context"
	"net/http"

	"github.com/kataras/iris"
)

const IrisCtxKey = "irisctx"

// FromStd convert std handler to iris.Handler, with iris context embedded
func FromStd(handler http.HandlerFunc) iris.Handler {
	return func(ctx iris.Context) {
		r2 := ctx.Request().WithContext(context.WithValue(ctx.Request().Context(), IrisCtxKey, ctx))
		handler(ctx.ResponseWriter(), r2)
	}
}

func getIrisCtxFromStdCtx(ctx context.Context) iris.Context {
	return ctx.Value(IrisCtxKey).(iris.Context)
}
