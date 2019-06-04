package ginMiddlewares_test

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	ginMiddlewares "github.com/Laisky/go-utils/gin-middlewares"

	"github.com/Laisky/go-utils"
)

func ExampleAuth() {
	cfg := ginMiddlewares.NewAuthCfg("f32lifj2f32fj")
	auth := ginMiddlewares.NewAuth(cfg)

	uid := "123"
	expiresAt := utils.UTCNow().Add(7 * 24 * time.Hour)
	payload := map[string]interface{}{"a": "b"}
	auth.GenerateToken(uid, expiresAt, payload)

	Server := gin.New()
	Server.Handle("ANY", "/authorized/", ginMiddlewares.FromStd(DemoHandle))
}

func DemoHandle(w http.ResponseWriter, r *http.Request) {
	// ginMiddlewares
	w.Write([]byte("hello"))
}
