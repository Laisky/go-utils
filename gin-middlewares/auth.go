package middlewares

import (
	"context"
	"time"

	"github.com/Laisky/zap"
	"github.com/pkg/errors"

	utils "github.com/Laisky/go-utils"
	"gopkg.in/mgo.v2/bson"
)

const (
	// AuthTokenName jwt token cookie name
	AuthTokenName = "token"
	// AuthUserIDCtxKey key of user ID in jwt token
	AuthUserIDCtxKey = "auth_uid"
	// AuthTokenAge how long live
	AuthTokenAge = 3600 * 24 * 7 // 7days
)

// Auth JWT cookie based token generator and validator.
// Cookie looks like <AuthTokenName>:`{<AuthUserIDCtxKey>: "xxxx"}`
type Auth struct {
	*AuthCfg
	j *utils.JWT
}

// AuthCfg configuration of Auth
type AuthCfg struct {
	Secret        string
	CookieExpires time.Duration
}

// NewAuthCfg return AuthCfg with default configuration
func NewAuthCfg(secret string) *AuthCfg {
	return &AuthCfg{
		Secret:        secret,
		CookieExpires: 7 * 24 * time.Hour,
	}
}

// NewAuth create new Auth with AuthCfg
func NewAuth(cfg *AuthCfg) (*Auth, error) {
	j, err := utils.NewJWT(utils.NewJWTCfg([]byte(cfg.Secret)))
	if err != nil {
		return nil, errors.Wrap(err, "try to create Auth got error")
	}

	a := &Auth{
		AuthCfg: cfg,
		j:       j,
	}
	return a, nil
}

// ValidateAndGetUID get token from request.ctx then validate and return userid
func (a *Auth) ValidateAndGetUID(ctx context.Context) (uid bson.ObjectId, err error) {
	var (
		token   string
		payload map[string]interface{}
	)
	if token, err = GetGinCtxFromStdCtx(ctx).Cookie(AuthTokenName); err != nil {
		return "", errors.New("jwt token not found")
	}

	if payload, err = a.j.Validate(token); err != nil {
		return "", errors.Wrap(err, "token invalidate")
	}

	uid = bson.ObjectIdHex(payload[a.j.JWTUserIDKey].(string))
	return uid, nil
}

// UserItf User model interface
type UserItf interface {
	GetPayload() map[string]interface{}
	GetID() string
}

// CookieCfg configuration of cookies
type CookieCfg struct {
	MaxAge           int // seconds
	Path, Host       string
	Secure, HTTPOnly bool
}

// NewCookieCfg create default cookie configuration
func NewCookieCfg() *CookieCfg {
	return &CookieCfg{
		MaxAge:   AuthTokenAge,
		Path:     "/",
		Secure:   false,
		HTTPOnly: false,
	}
}

// SetLoginCookie set jwt token to cookies
func (a *Auth) SetLoginCookie(ctx context.Context, user UserItf, cfg *CookieCfg) (err error) {
	utils.Logger.Info("user login", zap.String("user", user.GetID()))
	ctx2 := GetGinCtxFromStdCtx(ctx)
	var token string
	if token, err = a.j.GenerateToken(user.GetID(), utils.Clock.GetUTCNow().Add(a.CookieExpires), user.GetPayload()); err != nil {
		return errors.Wrap(err, "try to generate token got error")
	}

	if cfg == nil {
		cfg = NewCookieCfg()
	}
	if cfg.Host == "" {
		cfg.Host = ctx2.Request.Host
	}
	ctx2.SetCookie(AuthTokenName, token, cfg.MaxAge, cfg.Path, cfg.Host, cfg.Secure, cfg.HTTPOnly)
	return nil
}
