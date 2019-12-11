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
	// defaultAuthTokenName jwt token cookie name
	defaultAuthTokenName = "token"
	// defaultAuthUserIDCtxKey key of user ID in jwt token
	defaultAuthUserIDCtxKey           = "auth_uid"
	defaultAuthJWTTokenExpireDuration = 7 * 24 * time.Hour

	defaultAuthCookieMaxAge   = 3600 * 24 * 7 // 7days
	defaultAuthCookiePath     = "/"
	defaultAuthCookieSecure   = false
	defaultAuthCookieHTTPOnly = false
)

type authOption struct {
	jwtTokenExpireDuration time.Duration
}

// AuthOptFunc auth option
type AuthOptFunc func(*authOption)

// WithAuthCookieExpireDuration set auth cookie expiration
func WithAuthCookieExpireDuration(d time.Duration) AuthOptFunc {
	return func(opt *authOption) {
		opt.jwtTokenExpireDuration = d
	}
}

// Auth JWT cookie based token generator and validator.
// Cookie looks like <defaultAuthTokenName>:`{<defaultAuthUserIDCtxKey>: "xxxx"}`
type Auth struct {
	*authOption
	jwt *utils.JWT
}

// NewAuth create new Auth with AuthCfg
func NewAuth(secret []byte, opts ...AuthOptFunc) (a *Auth, err error) {
	var j *utils.JWT
	if j, err = utils.NewJWT(secret); err != nil {
		return nil, errors.Wrap(err, "try to create Auth got error")
	}

	opt := &authOption{
		jwtTokenExpireDuration: defaultAuthJWTTokenExpireDuration,
	}
	for _, optf := range opts {
		optf(opt)
	}

	a = &Auth{
		authOption: opt,
		jwt:        j,
	}
	return a, nil
}

// ValidateAndGetUID get token from request.ctx then validate and return userid
func (a *Auth) ValidateAndGetUID(ctx context.Context) (uid bson.ObjectId, err error) {
	var (
		token   string
		payload map[string]interface{}
	)
	if token, err = GetGinCtxFromStdCtx(ctx).Cookie(defaultAuthTokenName); err != nil {
		return "", errors.New("jwt token not found")
	}

	if payload, err = a.jwt.Validate(token); err != nil {
		return "", errors.Wrap(err, "token invalidate")
	}

	uid = bson.ObjectIdHex(payload[a.jwt.GetUserIDKey()].(string))
	return uid, nil
}

// UserItf User model interface
type UserItf interface {
	GetPayload() map[string]interface{}
	GetID() string
}

type authCookieOption struct {
	maxAge           int
	path, host       string
	secure, httpOnly bool
}

// AuthCookieOptFunc auth cookie options
type AuthCookieOptFunc func(*authCookieOption)

// WithAuthCookieMaxAge set auth cookie's maxAge
func WithAuthCookieMaxAge(maxAge int) AuthCookieOptFunc {
	return func(opt *authCookieOption) {
		opt.maxAge = maxAge
	}
}

// WithAuthCookiePath set auth cookie's path
func WithAuthCookiePath(path string) AuthCookieOptFunc {
	return func(opt *authCookieOption) {
		opt.path = path
	}
}

// WithAuthCookieSecure set auth cookie's secure
func WithAuthCookieSecure(secure bool) AuthCookieOptFunc {
	return func(opt *authCookieOption) {
		opt.secure = secure
	}
}

// WithAuthCookieHTTPOnly set auth cookie's HTTPOnly
func WithAuthCookieHTTPOnly(httpOnly bool) AuthCookieOptFunc {
	return func(opt *authCookieOption) {
		opt.httpOnly = httpOnly
	}
}

// WithAuthCookieHost set auth cookie's host
func WithAuthCookieHost(host string) AuthCookieOptFunc {
	return func(opt *authCookieOption) {
		opt.host = host
	}
}

// SetLoginCookie set jwt token to cookies
func (a *Auth) SetLoginCookie(ctx context.Context, user UserItf, opts ...AuthCookieOptFunc) (err error) {
	utils.Logger.Info("user login", zap.String("user", user.GetID()))
	ctx2 := GetGinCtxFromStdCtx(ctx)

	opt := &authCookieOption{
		maxAge:   defaultAuthCookieMaxAge,
		path:     defaultAuthCookiePath,
		secure:   defaultAuthCookieSecure,
		httpOnly: defaultAuthCookieHTTPOnly,
		host:     ctx2.Request.Host,
	}
	for _, optf := range opts {
		optf(opt)
	}

	var token string
	if token, err = a.jwt.GenerateToken(user.GetID(), utils.Clock.GetUTCNow().Add(a.jwtTokenExpireDuration), user.GetPayload()); err != nil {
		return errors.Wrap(err, "try to generate token got error")
	}

	ctx2.SetCookie(defaultAuthTokenName, token, opt.maxAge, opt.path, opt.host, opt.secure, opt.httpOnly)
	return nil
}
