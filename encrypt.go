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
	// JWTSigningMethod default method to signing
	JWTSigningMethod = jwt.SigningMethodHS512
)

const (
	// JWTUserIDKey default key of user_id stores in token payload
	JWTUserIDKey = "uid"
	// JWTExpiresAtKey default key of expires_at stores in token payload
	JWTExpiresAtKey = "exp"
)

type baseJWT struct {
	JWTSigningMethod              *jwt.SigningMethodHMAC
	JWTUserIDKey, JWTExpiresAtKey string
}

// JWT struct to generate and validate jwt tokens
//
// use a global uniform secret to signing all token.
type JWT struct {
	*JwtCfg
}

// JwtCfg configuration of JWT
type JwtCfg struct {
	baseJWT
	Secret []byte
}

// NewJWTCfg create new JwtCfg  with secret
func NewJWTCfg(secret []byte) *JwtCfg {
	return &JwtCfg{
		Secret: secret,
		baseJWT: baseJWT{
			JWTSigningMethod: JWTSigningMethod,
			JWTUserIDKey:     JWTUserIDKey,
			JWTExpiresAtKey:  JWTExpiresAtKey,
		},
	}
}

// NewJWT create new JWT with JwtCfg
func NewJWT(cfg *JwtCfg) (*JWT, error) {
	if len(cfg.Secret) == 0 {
		return nil, errors.New("jwtCfg.Secret should not be empty")
	}

	jwt.TimeFunc = Clock.GetUTCNow

	return &JWT{
		JwtCfg: cfg,
	}, nil
}

// GenerateToken generate JWT token with userID(interface{})
func (j *JWT) GenerateToken(userID interface{}, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	jwtPayload[j.JWTExpiresAtKey] = expiresAt.Unix()
	jwtPayload[j.JWTUserIDKey] = userID

	token := jwt.NewWithClaims(JWTSigningMethod, jwtPayload)
	if tokenStr, err = token.SignedString(j.Secret); err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// VerifyAndReplaceExp check expires and replace expires to time.Time if validated
func (j *JWT) VerifyAndReplaceExp(payload map[string]interface{}) (err error) {
	now := Clock.GetUTCNow().Unix()
	switch exp := payload[j.JWTExpiresAtKey].(type) {
	case float64:
		if int64(exp) > now {
			payload[j.JWTExpiresAtKey] = time.Unix(int64(exp), 0).UTC()
			return nil
		}
		err = fmt.Errorf("token expired")
	case oj.Number:
		v, _ := exp.Int64()
		if v > now {
			payload[j.JWTExpiresAtKey] = time.Unix(v, 0).UTC()
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
		if method, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || method != j.JWTSigningMethod {
			return nil, errors.New("JWT method not allowd")
		}
		return j.Secret, nil
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
	if _, ok = payload[j.JWTUserIDKey]; !ok {
		err = fmt.Errorf("token must contains `%v`", j.JWTUserIDKey)
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
	*DivideJWTCfg
}

// JWTUserModel load secret by uid
type JWTUserModel interface {
	GetUID() interface{}
	LoadSecretByUID(uid interface{}) ([]byte, error)
}

// DivideJWTCfg configuration
type DivideJWTCfg struct {
	baseJWT
}

// NewDivideJWTCfg create new JwtCfg  with secret
func NewDivideJWTCfg() *DivideJWTCfg {
	jwt.TimeFunc = Clock.GetUTCNow
	return &DivideJWTCfg{
		baseJWT: baseJWT{
			JWTSigningMethod: JWTSigningMethod,
			JWTUserIDKey:     JWTUserIDKey,
			JWTExpiresAtKey:  JWTExpiresAtKey,
		},
	}
}

// NewDivideJWT create new JWT with JwtCfg
func NewDivideJWT(cfg *DivideJWTCfg) (*DivideJWT, error) {
	if cfg.JWTUserIDKey == "" ||
		cfg.JWTExpiresAtKey == "" ||
		cfg.JWTSigningMethod == nil {
		return nil, fmt.Errorf("configuration error")
	}

	return &DivideJWT{
		DivideJWTCfg: cfg,
	}, nil
}

// GenerateToken generate JWT token.
// do not use `expires_at` & `uid` as keys.
func (j *DivideJWT) GenerateToken(user JWTUserModel, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	jwtPayload[j.JWTExpiresAtKey] = expiresAt.Unix()
	jwtPayload[j.JWTUserIDKey] = user.GetUID()

	token := jwt.NewWithClaims(JWTSigningMethod, jwtPayload)
	var secret []byte
	if secret, err = user.LoadSecretByUID(user.GetUID()); err != nil {
		Logger.Error("try to load jwt secret by uid got error",
			zap.Error(err),
			zap.String("uid", fmt.Sprint(user.GetUID())))
		return "", err
	}
	if tokenStr, err = token.SignedString(secret); err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// VerifyAndReplaceExp check expires and replace expires to time.Time if validated
func (j *DivideJWT) VerifyAndReplaceExp(payload jwt.MapClaims) (err error) {
	now := Clock.GetUTCNow().Unix()
	switch exp := payload[j.JWTExpiresAtKey].(type) {
	case float64:
		if int64(exp) > now {
			payload[j.JWTExpiresAtKey] = time.Unix(int64(exp), 0).UTC()
			return nil
		}
		err = fmt.Errorf("token expired")
	case oj.Number:
		v, _ := exp.Int64()
		if v > now {
			payload[j.JWTExpiresAtKey] = time.Unix(v, 0).UTC()
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
func (j *DivideJWT) Validate(user JWTUserModel, tokenStr string) (payload jwt.MapClaims, err error) {
	Logger.Debug("Validate for token", zap.String("tokenStr", tokenStr))
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if method, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || method != j.JWTSigningMethod {
			return nil, errors.New("JWT method not allowd")
		}
		return user.LoadSecretByUID(user.GetUID())
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
	if _, ok = payload[j.JWTUserIDKey]; !ok {
		err = fmt.Errorf("token must contains `%v`", j.JWTUserIDKey)
	}
	return payload, err
}
