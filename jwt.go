package utils

import (
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
)

var (
	// SignMethodHS256 use HS256 for jwt
	SignMethodHS256 = jwt.SigningMethodHS256
	// SignMethodES256 use ES256 for jwt
	SignMethodES256 = jwt.SigningMethodES256

	defaultSignMethod = SignMethodHS256
)

// ParseJWTTokenWithoutValidate parse and get payload without validate jwt token
func ParseJWTTokenWithoutValidate(token string, payload jwt.Claims) (err error) {
	_, _, err = new(jwt.Parser).ParseUnverified(token, payload)
	return err
}

// JWT is token utils that support HS256/ES256
type JWT struct {
	secret,
	priKey, pubKey []byte
	signingMethod jwt.SigningMethod
}

// JWTOptFunc options to setup JWT
type JWTOptFunc func(*JWT) error

// WithJWTSignMethod set jwt signing method
func WithJWTSignMethod(method jwt.SigningMethod) JWTOptFunc {
	return func(e *JWT) error {
		e.signingMethod = method
		return nil
	}
}

// WithJWTSecretByte set jwt symmetric signning key
func WithJWTSecretByte(secret []byte) JWTOptFunc {
	return func(e *JWT) error {
		e.secret = secret
		return nil
	}
}

// WithJWTPriKeyByte set jwt asymmetrical private key
func WithJWTPriKeyByte(prikey []byte) JWTOptFunc {
	return func(e *JWT) error {
		e.priKey = prikey
		return nil
	}
}

// WithJWTPubKeyByte set jwt asymmetrical public key
func WithJWTPubKeyByte(pubkey []byte) JWTOptFunc {
	return func(e *JWT) error {
		e.pubKey = pubkey
		return nil
	}
}

type jwtDivideOpt struct {
	priKey, pubKey,
	secret []byte
}

// JWTDiviceOptFunc options to use separate secret for every user in parsing/signing
type JWTDiviceOptFunc func(*jwtDivideOpt) error

// WithJWTDivideSecret set symmetric key for each signning/verify
func WithJWTDivideSecret(secret []byte) JWTDiviceOptFunc {
	return func(opt *jwtDivideOpt) error {
		opt.secret = secret
		return nil
	}
}

// WithJWTDividePriKey set asymmetrical private key for each signning/verify
func WithJWTDividePriKey(priKey []byte) JWTDiviceOptFunc {
	return func(opt *jwtDivideOpt) error {
		opt.priKey = priKey
		return nil
	}
}

// WithJWTDividePubKey set asymmetrical public key for each signning/verify
func WithJWTDividePubKey(pubKey []byte) JWTDiviceOptFunc {
	return func(opt *jwtDivideOpt) error {
		opt.pubKey = pubKey
		return nil
	}
}

// NewJWT create new JWT utils
func NewJWT(opts ...JWTOptFunc) (e *JWT, err error) {
	e = &JWT{
		signingMethod: defaultSignMethod,
	}

	for _, optf := range opts {
		if err = optf(e); err != nil {
			return nil, errors.Wrap(err, "apply option")
		}
	}

	return
}

// Sign sign claims to token
func (e *JWT) Sign(claims jwt.Claims, opts ...JWTDiviceOptFunc) (string, error) {
	switch e.signingMethod {
	case SignMethodHS256:
		return e.SignByHS256(claims, opts...)
	case SignMethodES256:
		return e.SignByES256(claims, opts...)
	}

	return "", errors.Errorf("unknown signmethod `%s`", e.signingMethod)
}

// SignByHS256 signing claims by HS256
func (e *JWT) SignByHS256(claims jwt.Claims, opts ...JWTDiviceOptFunc) (string, error) {
	opt := &jwtDivideOpt{
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
func (e *JWT) SignByES256(claims jwt.Claims, opts ...JWTDiviceOptFunc) (string, error) {
	opt := &jwtDivideOpt{
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
func (e *JWT) ParseClaims(token string, claimsPtr jwt.Claims, opts ...JWTDiviceOptFunc) error {
	if !IsPtr(claimsPtr) {
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
func (e *JWT) ParseClaimsByHS256(token string, claimsPtr jwt.Claims, opts ...JWTDiviceOptFunc) error {
	opt := &jwtDivideOpt{
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
func (e *JWT) ParseClaimsByES256(token string, claimsPtr jwt.Claims, opts ...JWTDiviceOptFunc) error {
	opt := &jwtDivideOpt{
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
