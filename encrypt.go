package utils

// JWT payload should looks like:
//
// ```js
// {
// 	"k1": "v1",
// 	"k2": "v2",
// 	"k3": "v3",
// 	"username": "laisky"
// }
// ```
//
// and the payload would be looks like:
//
// ```js
// {
// 	"expires_at": "2286-11-20T17:46:40Z",
// 	"k1": "v1",
// 	"k2": "v2",
// 	"k3": "v3",
// 	"username": "laisky"
//   }
// ```

import (
	"fmt"
	"time"

	"github.com/Laisky/zap"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"

	jwt "github.com/dgrijalva/jwt-go"
)

// JWT struct to generate and validate jwt tokens
type JWT struct {
	secret       []byte
	layout       string
	ExpiresAtKey string
	UserIDKey    string
}

// Setup initialize JWT
func (j *JWT) Setup(secret string) {
	// const key names
	j.ExpiresAtKey = "expires_at"
	j.UserIDKey = "uid"

	j.secret = []byte(secret)
	j.layout = time.RFC3339
}

// Deprecated: Generate generate JWT token.
// old interface
func (j *JWT) Generate(expiresAt int64, payload map[string]interface{}) (string, error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	jwtPayload["expires_at"] = ParseTs2String(expiresAt, j.layout)

	token := jwt.NewWithClaims(jwt.SigningMethodHS512, jwtPayload)
	tokenStr, err := token.SignedString(j.secret)
	if err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// GenerateToken generate JWT token.
// do not use `expires_at` & `uid` as keys.
func (j *JWT) GenerateToken(userId string, expiresAt time.Time, payload map[string]interface{}) (tokenStr string, err error) {
	jwtPayload := jwt.MapClaims{}
	for k, v := range payload {
		jwtPayload[k] = v
	}
	jwtPayload[j.ExpiresAtKey] = expiresAt.Format(j.layout)
	jwtPayload[j.UserIDKey] = userId

	fmt.Println(">>", jwtPayload)
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, jwtPayload)
	if tokenStr, err = token.SignedString(j.secret); err != nil {
		return "", errors.Wrap(err, "try to signed token got error")
	}
	return tokenStr, nil
}

// checkExpiresValid return the bool whether the `expires_at` is not expired
func (j *JWT) checkExpiresValid(now time.Time, expiresAtI interface{}) (ok bool, err error) {
	expiresAt, ok := expiresAtI.(string)
	if !ok {
		return false, fmt.Errorf("`%v` is not string", j.ExpiresAtKey)
	}
	tokenT, err := time.Parse(j.layout, expiresAt)
	if err != nil {
		return false, errors.Wrap(err, "try to parse token expires_at error")
	}

	return now.Before(tokenT), nil
}

// Validate validate the token and return the payload
func (j *JWT) Validate(tokenStr string) (payload map[string]interface{}, err error) {
	Logger.Debug("Validate for token", zap.String("tokenStr", tokenStr))
	payload = map[string]interface{}{}
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("JWT method not allowd")
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "token validate error")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		for k, v := range claims {
			payload[k] = v
		}
		if _, ok := payload[j.UserIDKey]; !ok {
			return payload, fmt.Errorf("token do not contains `%v`", j.UserIDKey)
		}

		if expiresAt, ok := payload[j.ExpiresAtKey]; !ok {
			return payload, fmt.Errorf("token do not contains `%v`", j.ExpiresAtKey)
		} else {
			if ok, err = j.checkExpiresValid(UTCNow(), expiresAt); err != nil {
				return payload, errors.Wrap(err, "parse token `expires_at` error")
			} else if !ok {
				return payload, fmt.Errorf("token expired at %v", payload[j.ExpiresAtKey])
			}
		}

		return payload, nil
	}
	return nil, errors.New("token not match MapClaims")
}

func GeneratePasswordHash(password []byte) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func ValidatePasswordHash(hashedPassword, password []byte) bool {
	return bcrypt.CompareHashAndPassword(hashedPassword, password) == nil
}
