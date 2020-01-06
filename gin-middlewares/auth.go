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
	// defaultAuthUserIDCtxKey           = "auth_uid"
	defaultAuthJWTTokenExpireDuration = 7 * 24 * time.Hour

	defaultAuthCookieMaxAge   = 3600 * 24 * 7 // 7days
	defaultAuthCookiePath     = "/"
	defaultAuthCookieSecure   = false
	defaultAuthCookieHTTPOnly = false
)

// AuthOptFunc auth option
type AuthOptFunc func(*Auth)

// WithAuthCookieExpireDuration set auth cookie expiration
func WithAuthCookieExpireDuration(d time.Duration) AuthOptFunc {
	if d < 0 {
		utils.Logger.Panic("duration should not less than 0", zap.Duration("duration", d))
	}
	return func(opt *Auth) {
		opt.jwtTokenExpireDuration = d
	}
}

// Auth JWT cookie based token generator and validator.
// Cookie looks like <defaultAuthTokenName>:`{<defaultAuthUserIDCtxKey>: "xxxx"}`
type Auth struct {
	jwt                    *utils.JWT
	jwtTokenExpireDuration time.Duration
}

// NewAuth create new Auth
func NewAuth(secret []byte, opts ...AuthOptFunc) (a *Auth, err error) {
	var j *utils.JWT
	if j, err = utils.NewJWT(secret); err != nil {
		return nil, errors.Wrap(err, "try to create Auth got error")
	}

	a = &Auth{
		jwtTokenExpireDuration: defaultAuthJWTTokenExpireDuration,
		jwt:                    j,
	}
	for _, optf := range opts {
		optf(a)
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
	if maxAge < 0 {
		utils.Logger.Panic("maxAge should not less than 0", zap.Int("maxAge", maxAge))
	}
	return func(opt *authCookieOption) {
		opt.maxAge = maxAge
	}
}

// WithAuthCookiePath set auth cookie's path
func WithAuthCookiePath(path string) AuthCookieOptFunc {
	utils.Logger.Debug("set auth cookie path", zap.String("path", path))
	return func(opt *authCookieOption) {
		opt.path = path
	}
}

// WithAuthCookieSecure set auth cookie's secure
func WithAuthCookieSecure(secure bool) AuthCookieOptFunc {
	utils.Logger.Debug("set auth cookie secure", zap.Bool("secure", secure))
	return func(opt *authCookieOption) {
		opt.secure = secure
	}
}

// WithAuthCookieHTTPOnly set auth cookie's HTTPOnly
func WithAuthCookieHTTPOnly(httpOnly bool) AuthCookieOptFunc {
	utils.Logger.Debug("set auth cookie httpOnly", zap.Bool("httpOnly", httpOnly))
	return func(opt *authCookieOption) {
		opt.httpOnly = httpOnly
	}
}

// WithAuthCookieHost set auth cookie's host
func WithAuthCookieHost(host string) AuthCookieOptFunc {
	utils.Logger.Debug("set auth cookie host", zap.String("host", host))
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
