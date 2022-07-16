// Package jwt all in one JWT sdk
package jwt

import (
	gutils "github.com/Laisky/go-utils/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
)

var (
	// SignMethodHS256 use HS256 for jwt
	SignMethodHS256 = jwt.SigningMethodHS256
	// SignMethodES256 use ES256 for jwt
	SignMethodES256 = jwt.SigningMethodES256
	SignMethodRS256 = jwt.SigningMethodRS256

	defaultSignMethod = SignMethodHS256
)

type JWT interface {
	Sign(claims jwt.Claims, opts ...DivideOption) (string, error)
	SignByHS256(claims jwt.Claims, opts ...DivideOption) (string, error)
	SignByES256(claims jwt.Claims, opts ...DivideOption) (string, error)
	ParseClaims(token string, claimsPtr jwt.Claims, opts ...DivideOption) error
	ParseClaimsByHS256(token string, claimsPtr jwt.Claims, opts ...DivideOption) error
	ParseClaimsByES256(token string, claimsPtr jwt.Claims, opts ...DivideOption) error
	ParseClaimsByRS256(token string, claimsPtr jwt.Claims, opts ...DivideOption) error
}

// ParseTokenWithoutValidate parse and get payload without validate jwt token
func ParseTokenWithoutValidate(token string, payload jwt.Claims) (err error) {
	_, _, err = new(jwt.Parser).ParseUnverified(token, payload)
	return err
}

// jwtType is token utils that support HS256/ES256
type jwtType struct {
	secret,
	priKey, pubKey []byte
	signingMethod jwt.SigningMethod
}

// Option options to setup JWT
type Option func(*jwtType) error

// WithSignMethod set jwt signing method
func WithSignMethod(method jwt.SigningMethod) Option {
	return func(e *jwtType) error {
		e.signingMethod = method
		return nil
	}
}

// WithSecretByte set jwt symmetric signning key
func WithSecretByte(secret []byte) Option {
	return func(e *jwtType) error {
		e.secret = secret
		return nil
	}
}

// WithPriKeyByte set jwt asymmetrical private key
func WithPriKeyByte(prikey []byte) Option {
	return func(e *jwtType) error {
		e.priKey = prikey
		return nil
	}
}

// WithPubKeyByte set jwt asymmetrical public key
func WithPubKeyByte(pubkey []byte) Option {
	return func(e *jwtType) error {
		e.pubKey = pubkey
		return nil
	}
}

type divideOpt struct {
	priKey, pubKey,
	secret []byte
}

// DivideOption options to use separate secret for every user in parsing/signing
type DivideOption func(*divideOpt) error

// WithDivideSecret set symmetric key for each signning/verify
func WithDivideSecret(secret []byte) DivideOption {
	return func(opt *divideOpt) error {
		opt.secret = secret
		return nil
	}
}

// WithDividePriKey set asymmetrical private key for each signning/verify
func WithDividePriKey(priKey []byte) DivideOption {
	return func(opt *divideOpt) error {
		opt.priKey = priKey
		return nil
	}
}

// WithDividePubKey set asymmetrical public key for each signning/verify
func WithDividePubKey(pubKey []byte) DivideOption {
	return func(opt *divideOpt) error {
		opt.pubKey = pubKey
		return nil
	}
}

// New create new JWT utils
func New(opts ...Option) (JWT, error) {
	e := &jwtType{
		signingMethod: defaultSignMethod,
	}

	for _, optf := range opts {
		if err := optf(e); err != nil {
			return nil, errors.Wrap(err, "apply option")
		}
	}

	return e, nil
}

// Sign sign claims to token
func (e *jwtType) Sign(claims jwt.Claims, opts ...DivideOption) (string, error) {
	switch e.signingMethod {
	case SignMethodHS256:
		return e.SignByHS256(claims, opts...)
	case SignMethodES256:
		return e.SignByES256(claims, opts...)
	}

	return "", errors.Errorf("unknown signmethod `%s`", e.signingMethod)
}

// SignByHS256 signing claims by HS256
func (e *jwtType) SignByHS256(claims jwt.Claims, opts ...DivideOption) (string, error) {
	opt := &divideOpt{
		secret: e.secret,
	}
	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return "", errors.Wrap(err, "apply optf")
		}
	}

	token := jwt.NewWithClaims(SignMethodHS256, claims)
	return token.SignedString(opt.secret)
}

// SignByES256 signing claims by ES256
func (e *jwtType) SignByES256(claims jwt.Claims, opts ...DivideOption) (string, error) {
	opt := &divideOpt{
		pubKey: e.pubKey,
		priKey: e.priKey,
	}
	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return "", errors.Wrap(err, "apply optf")
		}
	}

	token := jwt.NewWithClaims(SignMethodES256, claims)
	priKey, err := jwt.ParseECPrivateKeyFromPEM(opt.priKey)
	if err != nil {
		return "", errors.Wrap(err, "parse private key")
	}

	return token.SignedString(priKey)
}

// ParseClaims parse token to claims
func (e *jwtType) ParseClaims(token string, claimsPtr jwt.Claims, opts ...DivideOption) error {
	if !gutils.IsPtr(claimsPtr) {
		return errors.New("claimsPtr must be a pointer")
	}

	switch e.signingMethod {
	case SignMethodHS256:
		return e.ParseClaimsByHS256(token, claimsPtr, opts...)
	case SignMethodES256:
		return e.ParseClaimsByES256(token, claimsPtr, opts...)
	default:
		return errors.Errorf("unknown sign method `%s`", e.signingMethod)
	}
}

// ParseClaimsByHS256 parse token to claims by HS256
func (e *jwtType) ParseClaimsByHS256(token string, claimsPtr jwt.Claims, opts ...DivideOption) error {
	opt := &divideOpt{
		secret: e.secret,
	}
	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return errors.Wrap(err, "apply optf")
		}
	}

	if _, err := jwt.ParseWithClaims(token, claimsPtr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return opt.secret, nil
	}); err != nil {
		return errors.Wrap(err, "parse token by hs256")
	}

	return nil
}

// ParseClaimsByES256 parse token to claims by ES256
func (e *jwtType) ParseClaimsByES256(token string, claimsPtr jwt.Claims, opts ...DivideOption) error {
	opt := &divideOpt{
		pubKey: e.pubKey,
		priKey: e.priKey,
	}
	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return errors.Wrap(err, "apply optf")
		}
	}

	pubKey, err := jwt.ParseECPublicKeyFromPEM(opt.pubKey)
	if err != nil {
		return errors.Wrap(err, "parse es256 public key")
	}

	if _, err = jwt.ParseWithClaims(token, claimsPtr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return pubKey, nil
	}); err != nil {
		return errors.Wrap(err, "parse token by es256")
	}

	return nil
}

// ParseClaimsByRS256 parse token to claims by rs256
func (e *jwtType) ParseClaimsByRS256(token string, claimsPtr jwt.Claims, opts ...DivideOption) error {
	opt := &divideOpt{
		pubKey: e.pubKey,
		priKey: e.priKey,
	}
	for _, optf := range opts {
		if err := optf(opt); err != nil {
			return errors.Wrap(err, "apply optf")
		}
	}

	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(opt.pubKey)
	if err != nil {
		return errors.Wrap(err, "parse rs256 public key")
	}

	if _, err = jwt.ParseWithClaims(token, claimsPtr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return pubKey, nil
	}); err != nil {
		return errors.Wrap(err, "parse token by rs256")
	}

	return nil
}
