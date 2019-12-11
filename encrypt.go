package utils

// JWT payload should looks like:
//
// ```js
// {
// 	"k1": "v1",
// 	"k2": "v2",
// 	"k3": "v3",
// 	"uid": "laisky"
// }
// ```
//
// and the payload would be looks like:
//
// ```js
// {
//     "uid": "laisky",
// 	   "exp": 4701974400
// }
// ```

import (
	oj "encoding/json"
	"fmt"
	"time"

	"github.com/Laisky/zap"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"

	jwt "github.com/dgrijalva/jwt-go"
)

var (
	// defaultJWTSignMethod default jwt signing method
	defaultJWTSignMethod = jwt.SigningMethodHS512
	// defaultJWTUserIDKey default key of user_id stores in token payload
	defaultJWTUserIDKey = "uid"
	// defaultJWTExpiresKey default key of expires_at stores in token payload
	defaultJWTExpiresKey = "exp"
)

type baseJWT struct {
	JWTSigningMethod              *jwt.SigningMethodHMAC
	JWTUserIDKey, JWTExpiresAtKey string
}

// JWT struct to generate and validate jwt tokens
//
// use a global uniform secret to signing all token.
type JWT struct {
	*jwtOption
	secret []byte
}

type jwtOption struct {
	signMethod *jwt.SigningMethodHMAC
	userIDKey,
	expiresKey string
}

// JWTOptFunc jwt option
type JWTOptFunc func(*jwtOption)

// WithJWTSignMethod set jwt sign method
func WithJWTSignMethod(method *jwt.SigningMethodHMAC) JWTOptFunc {
	if method == nil {
		Logger.Panic("method should not be nil")
	}
	return func(opt *jwtOption) {
		opt.signMethod = method
	}
}

// WithJWTUserIDKey set jwt user id key in payload
func WithJWTUserIDKey(userIDKey string) JWTOptFunc {
	if userIDKey == "" {
		Logger.Panic("userIDKey should not be empty")
	}
	return func(opt *jwtOption) {
		opt.userIDKey = userIDKey
		if opt.expiresKey == opt.userIDKey {
			Logger.Panic("expiresKey should not equal to userIDKey")
		}
	}
}

// WithJWTExpiresKey set jwt expires key in payload
func WithJWTExpiresKey(expiresKey string) JWTOptFunc {
	if expiresKey == "" {
		Logger.Panic("expiresKey should not be empty")
	}
	return func(opt *jwtOption) {
		opt.expiresKey = expiresKey
		if opt.expiresKey == opt.userIDKey {
			Logger.Panic("expiresKey should not equal to userIDKey")
		}
	}
}

// NewJWT create new JWT with JwtCfg
func NewJWT(secret []byte, opts ...JWTOptFunc) (*JWT, error) {
	if len(secret) == 0 {
		return nil, errors.New("jwtCfg.Secret should not be empty")
	}
	opt := &jwtOption{
		signMethod: defaultJWTSignMethod,
		userIDKey:  defaultJWTUserIDKey,
		expiresKey: defaultJWTExpiresKey,
	}
	for _, optf := range opts {
		optf(opt)
	}

	jwt.TimeFunc = Clock.GetUTCNow
	return &JWT{
		jwtOption: opt,
		secret:    secret,
	}, nil
}

// GetSignMethod get jwt sign method
func (j *JWT) GetSignMethod() *jwt.SigningMethodHMAC {
	return j.signMethod
}

// GetUserIDKey get jwt user id key
func (j *JWT) GetUserIDKey() string {
	return j.userIDKey
}

// GetExpiresKey get jwt expires key
func (j *JWT) GetExpiresKey() string {
	return j.expiresKey
}

// GenerateToken generate JWT token with userID(interface{})
func (j *JWT) GenerateToken(userID interface{}, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	jwtPayload[j.expiresKey] = expiresAt.Unix()
	jwtPayload[j.userIDKey] = userID

	token := jwt.NewWithClaims(j.signMethod, jwtPayload)
	if tokenStr, err = token.SignedString(j.secret); err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// VerifyAndReplaceExp check expires and replace expires to time.Time if validated
func (j *JWT) VerifyAndReplaceExp(payload map[string]interface{}) (err error) {
	now := Clock.GetUTCNow().Unix()
	switch exp := payload[j.expiresKey].(type) {
	case float64:
		if int64(exp) > now {
			payload[j.expiresKey] = time.Unix(int64(exp), 0).UTC()
			return nil
		}
		err = fmt.Errorf("token expired")
	case oj.Number:
		v, _ := exp.Int64()
		if v > now {
			payload[j.expiresKey] = time.Unix(v, 0).UTC()
			return nil
		}
		err = fmt.Errorf("token expired")
	default:
		err = fmt.Errorf("unknown expires format")
	}

	return err
}

// Validate validate the token and return the payload
//
// if token is invalidate, err will not be nil.
func (j *JWT) Validate(tokenStr string) (payload jwt.MapClaims, err error) {
	Logger.Debug("Validate for token", zap.String("tokenStr", tokenStr))
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if method, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || method != j.signMethod {
			return nil, errors.New("JWT method not allowd")
		}
		return j.secret, nil
	})
	if err != nil || !token.Valid {
		// return after got payload
		err = errors.Wrap(err, "token invalidate")
	}

	var ok bool
	if payload, ok = token.Claims.(jwt.MapClaims); !ok {
		return nil, errors.New("payload type not match `map[string]interface{}`")
	}
	if err != nil {
		return payload, err
	}

	if err = j.VerifyAndReplaceExp(payload); err != nil { // exp must exists
		return payload, errors.Wrap(err, "token invalidate")
	}
	if _, ok = payload[j.userIDKey]; !ok {
		err = fmt.Errorf("token must contains `%v`", j.userIDKey)
	}
	return payload, err
}

// GeneratePasswordHash generate hashed password by origin password
func GeneratePasswordHash(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// ValidatePasswordHash validate password is match with hashedPassword
func ValidatePasswordHash(hashedPassword, password []byte) bool {
	return bcrypt.CompareHashAndPassword(hashedPassword, password) == nil
}

// DivideJWT jwt utils to generate and validate token.
//
// use seperate secret for each token
type DivideJWT struct {
	*jwtOption
}

// JWTUserItf load secret by uid
type JWTUserItf interface {
	GetUID() interface{}
	GetSecret() []byte
}

// NewDivideJWT create new JWT with JwtCfg
func NewDivideJWT(opts ...JWTOptFunc) (*DivideJWT, error) {
	opt := &jwtOption{
		signMethod: defaultJWTSignMethod,
		userIDKey:  defaultJWTUserIDKey,
		expiresKey: defaultJWTExpiresKey,
	}
	for _, optf := range opts {
		optf(opt)
	}

	jwt.TimeFunc = Clock.GetUTCNow
	return &DivideJWT{
		jwtOption: opt,
	}, nil
}

// GetSignMethod get jwt sign method
func (j *DivideJWT) GetSignMethod() *jwt.SigningMethodHMAC {
	return j.signMethod
}

// GetUserIDKey get jwt user id key
func (j *DivideJWT) GetUserIDKey() string {
	return j.userIDKey
}

// GetExpiresKey get jwt expires key
func (j *DivideJWT) GetExpiresKey() string {
	return j.expiresKey
}

// GenerateToken generate JWT token.
// do not use `expires_at` & `uid` as keys.
func (j *DivideJWT) GenerateToken(user JWTUserItf, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error) {
	jwtPayload := jwt.MapClaims{}
	if payload != nil {
		for k, v := range payload {
			jwtPayload[k] = v
		}
	}
	jwtPayload[j.expiresKey] = expiresAt.Unix()
	jwtPayload[j.userIDKey] = user.GetUID()

	token := jwt.NewWithClaims(j.signMethod, jwtPayload)
	if tokenStr, err = token.SignedString(user.GetSecret()); err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// VerifyAndReplaceExp check expires and replace expires to time.Time if validated
func (j *DivideJWT) VerifyAndReplaceExp(payload jwt.MapClaims) (err error) {
	now := Clock.GetUTCNow().Unix()
	switch exp := payload[j.expiresKey].(type) {
	case float64:
		if int64(exp) > now {
			payload[j.expiresKey] = time.Unix(int64(exp), 0).UTC()
			return nil
		}
		err = fmt.Errorf("token expired")
	case oj.Number:
		v, _ := exp.Int64()
		if v > now {
			payload[j.expiresKey] = time.Unix(v, 0).UTC()
			return nil
		}
		err = fmt.Errorf("token expired")
	default:
		err = fmt.Errorf("unknown expires format")
	}

	return err
}

// Validate validate the token and return the payload
//
// if token is invalidate, err will not be nil.
func (j *DivideJWT) Validate(user JWTUserItf, tokenStr string) (payload jwt.MapClaims, err error) {
	Logger.Debug("Validate for token", zap.String("tokenStr", tokenStr))
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if method, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || method != j.signMethod {
			return nil, errors.New("JWT method not allowd")
		}
		return user.GetSecret(), nil
	})
	if err != nil || !token.Valid {
		// return after got payload
		err = errors.Wrap(err, "token invalidate")
	}

	var ok bool
	if payload, ok = token.Claims.(jwt.MapClaims); !ok {
		return nil, errors.New("payload type not match `map[string]interface{}`")
	}
	if err != nil {
		return payload, err
	}

	if err = j.VerifyAndReplaceExp(payload); err != nil { // exp must exists
		return payload, errors.Wrap(err, "token invalidate")
	}
	if _, ok = payload[j.userIDKey]; !ok {
		err = fmt.Errorf("token must contains `%v`", j.userIDKey)
	}
	return payload, err
}
