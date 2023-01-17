package encrypt

import (
	"crypto/sha1"
	"encoding/base32"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors"
	"github.com/xlzd/gotp"

	gutils "github.com/Laisky/go-utils/v3"
)

// OTPType otp type
type OTPType string

const (
	// OTPTypeTOTP time-based otp
	OTPTypeTOTP OTPType = "totp"
	// OTPTypeHOTP hash-based otp
	OTPTypeHOTP OTPType = "hotp"
)

// OTPAlgorithm hash algorithm for otp
type OTPAlgorithm string

const (
	// OTPAlgorithmSHA1 sha1
	OTPAlgorithmSHA1 OTPAlgorithm = "sha1"
)

// Base32Secret generate base32 encoded secret
func Base32Secret(key []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key)
}

// OTPArgs arguments for OTP
type OTPArgs struct {
	// OtpType (readonly) otp type, must in totp/hotp
	OtpType OTPType
	// Base32Secret (required) the hotp/totp secret used to generate the URI
	//
	// genrate Base32Secret:
	//
	//  Base32Secret([]byte("xxxxxx"))
	Base32Secret string
	// AccountName (optional) name of the account
	AccountName string
	// Authenticator (optional) the name of the OTP issuer;
	// this will be the organization title of the OTP entry in Authenticator
	IssuerName string
	// Algorithm (optional) the algorithm used in the OTP generation
	//
	// default to sha1
	Algorithm OTPAlgorithm
	// InitialCount (optional) starting counter value. Only works for hotp
	InitialCount int
	// Digits (optional) the length of the OTP generated code.
	//
	// default to 6
	Digits uint
	// PeriodSecs (optional) the number of seconds the OTP
	// generator is set to expire every code.
	//
	// default to 30
	PeriodSecs uint
}

// Hasher get hasher from argument
func (a OTPArgs) Hasher() (*gotp.Hasher, error) {
	switch strings.ToLower(string(a.Algorithm)) {
	case "", string(OTPAlgorithmSHA1):
		return &gotp.Hasher{
			HashName: string(OTPAlgorithmSHA1),
			Digest:   sha1.New,
		}, nil
	default:
		return nil, errors.Errorf("unsupport hasher %q", a.Algorithm)
	}
}

// TOTP time-based OTP
type TOTP struct {
	arg    OTPArgs
	engine *gotp.TOTP
}

// TOTPInterface interface for TOTP
type TOTPInterface interface {
	// Key generate key by totp
	Key() string
	// KeyAt generate key by totp at arbitraty time
	KeyAt(at time.Time) string
	// URI build uri for otp arguments
	URI() string
}

// NewTOTP new TOTP
func NewTOTP(arg OTPArgs) (*TOTP, error) {
	arg.OtpType = OTPTypeTOTP
	if len(arg.Base32Secret) == 0 {
		return nil, errors.Errorf("secret shoule not be empty")
	}

	arg.Algorithm = gutils.OptionalVal(&arg.Algorithm, OTPAlgorithmSHA1)
	arg.Digits = gutils.OptionalVal(&arg.Digits, 6)
	arg.PeriodSecs = gutils.OptionalVal(&arg.PeriodSecs, 30)

	hasher, err := arg.Hasher()
	if err != nil {
		return nil, err
	}

	return &TOTP{
		engine: gotp.NewTOTP(
			arg.Base32Secret,
			int(arg.Digits),
			int(arg.PeriodSecs),
			hasher,
		),
		arg: arg,
	}, nil
}

// Key generate key by totp
func (t *TOTP) Key() string {
	return t.engine.Now()
}

// KeyAt generate key by totp at arbitraty time
func (t *TOTP) KeyAt(at time.Time) string {
	return t.engine.AtTime(at)
}

// URI build uri for otp arguments
func (t *TOTP) URI() string {
	return gotp.BuildUri(
		string(t.arg.OtpType),
		t.arg.Base32Secret,
		t.arg.AccountName,
		t.arg.IssuerName,
		string(t.arg.Algorithm),
		t.arg.InitialCount,
		int(t.arg.Digits),
		int(t.arg.PeriodSecs),
	)
}

// ParseOTPUri parse otp uri to otp arguments
//
// # Args
//
//   - uri: like `otpauth://totp/issuerName:demoAccountName?secret=4S62BZNFXXSZLCRO&issuer=issuerName`
func ParseOTPUri(uri string) (arg OTPArgs, err error) {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return arg, errors.Wrap(err, "parse uri")
	}

	arg.OtpType = OTPType(parsedURL.Host)
	arg.AccountName, err = url.PathUnescape(parsedURL.Path)
	if err != nil {
		return arg, errors.Wrapf(err, "unescape path %q", parsedURL.Path)
	}
	if vs := strings.SplitN(arg.AccountName, ":", 2); len(vs) == 2 {
		arg.IssuerName = strings.TrimPrefix(vs[0], "/")
		arg.AccountName = vs[1]
	}

	arg.Base32Secret, err = url.QueryUnescape(parsedURL.Query().Get("secret"))
	if err != nil {
		return arg, errors.Wrap(err, "unescape secret")
	}

	digit, err := url.QueryUnescape(parsedURL.Query().Get("digit"))
	if err != nil {
		return arg, errors.Wrap(err, "unescape digit")
	}
	if gutils.Contains([]string{"0", "6", ""}, digit) {
		arg.Digits = 6
	} else {
		v, err := strconv.Atoi(digit)
		if err != nil {
			return arg, errors.Wrapf(err, "parse digit %q", digit)
		}

		arg.Digits = uint(v)
	}

	period, err := url.QueryUnescape(parsedURL.Query().Get("period"))
	if err != nil {
		return arg, errors.Wrap(err, "unescape period")
	}
	if gutils.Contains([]string{"0", "30", ""}, period) {
		arg.PeriodSecs = 30
	} else {
		v, err := strconv.Atoi(period)
		if err != nil {
			return arg, errors.Wrapf(err, "parse period %q", period)
		}

		arg.PeriodSecs = uint(v)
	}

	if arg.OtpType == "hotp" {
		counter, err := url.QueryUnescape(parsedURL.Query().Get("counter"))
		if err != nil {
			return arg, errors.Wrap(err, "unescape counter")
		}

		arg.InitialCount, err = strconv.Atoi(counter)
		if err != nil {
			return arg, errors.Wrapf(err, "parse counter %q", counter)
		}
	}

	arg.Algorithm = gutils.OptionalVal(&arg.Algorithm, OTPAlgorithmSHA1)

	return arg, nil
}
