package irisMiddlewares_test

import (
	"github.com/kataras/iris"
	"net/http"
	"github.com/Laisky/go-utils/irisMiddlewares"
)

func ExampleAuth() {
	irisMiddlewares.SetupAuth("SECRET_KEY")

	Server := iris.New()
	Server.Handle("ANY", "/authorized/", irisMiddlewares.FromStd(DemoHandle))
}

func DemoHandle(w http.ResponseWriter, r *http.Request) {
	// irisMiddlewares
	w.Write([]byte("hello"))
}
