package email

import (
	"net/smtp"
	"strings"

	"github.com/Laisky/errors/v2"
)

// selectSMTPAuth selects a supported mechanism advertised after TLS, preserving
// the historical CRAM-MD5, PLAIN, then LOGIN preference. Unknown mechanisms fail
// before credentials are sent; a failed authentication never retries another one.
func selectSMTPAuth(advertised, username, password, host string) (smtp.Auth, error) {
	supported := make(map[string]bool)
	for _, mechanism := range strings.Fields(advertised) {
		supported[strings.ToUpper(mechanism)] = true
	}
	switch {
	case supported["CRAM-MD5"]:
		return smtp.CRAMMD5Auth(username, password), nil
	case supported["PLAIN"]:
		return smtp.PlainAuth("", username, password, host), nil
	case supported["LOGIN"]:
		return &smtpLoginAuth{username: username, password: password, host: host}, nil
	default:
		return nil, errors.New("smtp server advertises no supported authentication mechanism")
	}
}

// smtpLoginAuth implements the historical LOGIN exchange, restricted to a verified TLS session.
type smtpLoginAuth struct {
	username, password, host string
	step                     int
}

// Start checks the TLS and peer-name boundary before initiating LOGIN without an initial credential response.
func (a *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server == nil || !server.TLS || server.Name != a.host {
		return "", nil, errors.New("SMTP LOGIN requires TLS to the configured server")
	}
	a.step = 0
	return "LOGIN", nil, nil
}

// Next returns one credential only for the expected bounded LOGIN challenge sequence.
func (a *smtpLoginAuth) Next(challenge []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(challenge)))
	switch {
	case a.step == 0 && prompt == "username:":
		a.step++
		return []byte(a.username), nil
	case a.step == 1 && prompt == "password:":
		a.step++
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected SMTP LOGIN challenge")
	}
}
