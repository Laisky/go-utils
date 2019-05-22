package irisMiddlewares

import (
	"context"
	"time"

	"github.com/Laisky/zap"
	"github.com/kataras/iris"
	"github.com/pkg/errors"

	utils "github.com/Laisky/go-utils"
	"gopkg.in/mgo.v2/bson"
)

const (
	AuthTokenName    = "token"
	AuthUserIdCtxKey = "auth_uid"
)

type Auth struct {
	utils.JWT
	*authCfgType
}

type authCfgType struct {
	Secret        string
	CookieExpires time.Duration
}

var AuthCfg = &authCfgType{
	CookieExpires: 7 * 24 * time.Hour,
}

func NewAuth(cfg *authCfgType) *Auth {
	a := &Auth{
		authCfgType: cfg,
	}
	a.Setup(cfg.Secret)
	return a
}

func (a *Auth) ValidateAndGetUID(ctx context.Context) (uid bson.ObjectId, err error) {
	token := GetIrisCtxFromStdCtx(ctx).GetCookie(AuthTokenName)
	payload, err := a.Validate(token)
	if err != nil {
		return "", errors.Wrap(err, "token invalidate")
	}

	uid = bson.ObjectIdHex(payload[a.UserIDKey].(string))
	return uid, nil
}

type UserItf interface {
	GetPayload() map[string]interface{}
	GetID() string
}

// SetLoginCookie set jwt token to cookies
func (a *Auth) SetLoginCookie(ctx context.Context, user UserItf) (err error) {
	utils.Logger.Info("user login", zap.String("user", user.GetID()))
	ctx2 := GetIrisCtxFromStdCtx(ctx)
	var token string
	if token, err = a.GenerateToken(user.GetID(), utils.Clock.GetUTCNow().Add(a.CookieExpires), user.GetPayload()); err != nil {
		return errors.Wrap(err, "try to generate token got error")
	}

	ctx2.SetCookieKV(AuthTokenName, token, iris.CookieExpires(a.CookieExpires))
	return nil
}
