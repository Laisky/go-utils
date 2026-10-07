// Package email is a simple SMTP email sender.
//
// # Transport security
//
// MailT.Send requires an encrypted, verified SMTP session by default. On the
// conventional implicit-TLS (SMTPS) port 465 the connection is wrapped in TLS
// before the SMTP greeting; on every other port the server must advertise
// STARTTLS and the upgrade must succeed. If encryption cannot be established,
// Send fails before any AUTH, MAIL, RCPT, or DATA command is transmitted, so
// neither credentials nor message content ever travel in cleartext. This
// defeats STARTTLS-stripping downgrade attacks.
//
// The default TLS configuration verifies the server certificate against the
// system roots, checks it against the configured SMTP host name, and requires
// TLS 1.2 or newer. WithEmailRequireTLS accepts a custom *tls.Config (for
// example a private CA pool); it is cloned per send, its ServerName defaults
// to the SMTP host when empty, and its MinVersion is raised to TLS 1.2 when
// lower. Any InsecureSkipVerify or VerifyConnection setting in a caller
// supplied config is honored as given and is the caller's responsibility.
//
// Migration: before issue #58 the zero-option default was gomail's
// opportunistic STARTTLS, which silently continued in plaintext when the
// server did not advertise STARTTLS. Operators of plaintext-only relays must
// now opt out explicitly with WithEmailInsecureAllowPlaintext, which restores
// the old opportunistic behavior. When both WithEmailRequireTLS and
// WithEmailInsecureAllowPlaintext are supplied, the strict mode wins.
//
// WithMailSendDialer replaces the transport entirely; a custom dialer factory
// takes precedence over every TLS option and is responsible for its own
// transport security.
package email

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
	gomail "gopkg.in/gomail.v2"

	"github.com/Laisky/go-utils/v6/log"
)

const (
	// smtpDialTimeout bounds how long the TLS-enforcing sender waits to establish
	// the TCP connection to the SMTP server.
	smtpDialTimeout = 30 * time.Second
	// smtpsPort is the conventional implicit-TLS (SMTPS) port. Connections to it
	// are wrapped in TLS before the SMTP greeting instead of using STARTTLS.
	smtpsPort = 465
)

// validateHeaderValue rejects values containing CRLF sequences
// to prevent email header injection attacks.
func validateHeaderValue(field, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return errors.Errorf("%s contains invalid characters (possible header injection)", field)
	}
	return nil
}

// maskUsername redacts a SMTP login identifier before it is written to logs,
// preventing credential/PII (the account name) from leaking into log sinks.
// For an email-style username only the domain part is preserved (the local
// part, which identifies the account holder, is masked); any other value is
// fully masked. An empty input yields an empty string so callers can detect
// "no auth configured".
func maskUsername(username string) string {
	if username == "" {
		return ""
	}

	// keep only the domain for email-style usernames; everything that could
	// identify the account holder is replaced with a fixed marker.
	if at := strings.LastIndex(username, "@"); at > 0 && at < len(username)-1 {
		return "***@" + username[at+1:]
	}

	return "***"
}

// Mail is a simple email sender
type Mail interface {
	// Login login to SMTP server
	Login(username, password string)
	// Send send email
	Send(frAddr, toAddr, frName, toName, subject, content string, optfs ...SendOption) (err error)
}

// MailT easy way to send basic email
type MailT struct {
	host               string
	port               int
	username, password string
}

// NewMail create Mail with SMTP host and port
func NewMail(host string, port int) *MailT {
	log.Shared.Debug("try to send mail", zap.String("host", host), zap.Int("port", port))
	return &MailT{
		host: host,
		port: port,
	}
}

// Login login to SMTP server
func (m *MailT) Login(username, password string) {
	// Do not log the raw username: it is an account identifier (often an
	// email address) and counts as credential/PII that must not reach log
	// sinks. Log a masked value instead so the debug line stays useful.
	log.Shared.Debug("login", zap.String("username", maskUsername(username)))
	m.username = username
	m.password = password
}

// BuildMessage implement
func (m *MailT) BuildMessage(msg string) string {
	return msg
}

// Sender create gomail.Dialer
type Sender interface {
	DialAndSend(m ...*gomail.Message) error
}

// mailSendOpt stores the per-call options resolved for MailT.Send.
type mailSendOpt struct {
	dialerFact func(host string, port int, username, passwd string) Sender
	// requireTLS records an explicit WithEmailRequireTLS; it overrides
	// allowPlaintext so conflicting options fail closed.
	requireTLS bool
	// allowPlaintext records WithEmailInsecureAllowPlaintext, the explicit
	// opt-out that restores gomail's opportunistic STARTTLS.
	allowPlaintext bool
	tlsConfig      *tls.Config
}

// fillDefault installs the default dialer factory and returns o. The factory
// reads the option fields when it is invoked (after applyOpts), selecting the
// TLS-enforcing requireTLSSender unless the caller explicitly opted out of
// transport security with WithEmailInsecureAllowPlaintext.
func (o *mailSendOpt) fillDefault() *mailSendOpt {
	o.dialerFact = func(host string, port int, username, passwd string) Sender {
		// Security: plaintext-capable delivery is only reachable through the
		// explicit insecure opt-out, and an explicit WithEmailRequireTLS always
		// wins so conflicting options fail closed.
		if o.allowPlaintext && !o.requireTLS {
			log.Shared.Debug("smtp transport security disabled by explicit opt-out",
				zap.String("host", host), zap.Int("port", port))
			return gomail.NewDialer(host, port, username, passwd)
		}

		implicitTLS := port == smtpsPort
		log.Shared.Debug("smtp transport requires tls",
			zap.String("host", host), zap.Int("port", port),
			zap.Bool("implicit_tls", implicitTLS),
			zap.Bool("custom_tls_config", o.tlsConfig != nil))
		return &requireTLSSender{
			host:        host,
			port:        port,
			username:    username,
			password:    passwd,
			implicitTLS: implicitTLS,
			tlsConfig:   o.tlsConfig,
		}
	}

	return o
}

// applyOpts applies every SendOption in order to o and returns o.
func (o *mailSendOpt) applyOpts(optfs []SendOption) *mailSendOpt {
	for _, optf := range optfs {
		optf(o)
	}
	return o
}

// SendOption is a function to set option for Mail.Send
type SendOption func(*mailSendOpt)

// WithMailSendDialer replaces the SMTP transport with a caller-supplied dialer
// factory, which receives the host, port, and credentials and returns the
// Sender used for delivery.
//
// Security: the supplied factory takes precedence over WithEmailRequireTLS
// and WithEmailInsecureAllowPlaintext, so it fully owns transport security.
// The returned Sender must itself refuse plaintext delivery when that is
// required.
func WithMailSendDialer(dialerFact func(host string, port int, username, passwd string) Sender) SendOption {
	return func(opt *mailSendOpt) {
		opt.dialerFact = dialerFact
	}
}

// WithEmailRequireTLS enforces an encrypted SMTP connection and refuses to fall
// back to plaintext.
//
// Encrypted delivery is already the default (see the package documentation),
// so this option is only needed to supply a custom *tls.Config, or to make the
// strict mode explicit: it overrides WithEmailInsecureAllowPlaintext regardless
// of option order. For the implicit-TLS port 465 Send dials directly over TLS;
// for any other port it requires the server to advertise STARTTLS and aborts
// WITHOUT sending credentials or message data if it does not.
//
// An optional *tls.Config may be supplied. When nil, a config using the SMTP
// host as ServerName, the system roots, and a TLS 1.2 minimum is used. A
// supplied config is cloned per send; its ServerName defaults to the SMTP host
// when empty and its MinVersion is raised to TLS 1.2 when lower.
//
// If WithMailSendDialer is also provided, the explicitly supplied dialer takes
// precedence and this option has no effect.
func WithEmailRequireTLS(tlsConfig ...*tls.Config) SendOption {
	return func(opt *mailSendOpt) {
		opt.requireTLS = true
		if len(tlsConfig) > 0 {
			opt.tlsConfig = tlsConfig[0]
		}
	}
}

// WithEmailInsecureAllowPlaintext is an explicit, insecure opt-out that
// restores the legacy opportunistic STARTTLS behavior of gomail's dialer.
//
// Security: with this option a server that does not advertise STARTTLS (for
// example because an on-path attacker stripped the capability) receives the
// message, and possibly credentials for mechanisms gomail permits without TLS,
// in cleartext. Use it only for trusted plaintext-only relays (such as a local
// development relay) and migrate to an encrypted relay as soon as possible.
// WithEmailRequireTLS overrides this option, and WithMailSendDialer replaces
// the transport entirely.
func WithEmailInsecureAllowPlaintext() SendOption {
	return func(opt *mailSendOpt) {
		opt.allowPlaintext = true
	}
}

// requireTLSSender is a Sender that guarantees the SMTP session is encrypted
// before any credentials or message data are transmitted, defending against
// STARTTLS-stripping downgrade attacks.
type requireTLSSender struct {
	host, username, password string
	port                     int
	// implicitTLS wraps the TCP connection in TLS before the SMTP greeting
	// (SMTPS); otherwise STARTTLS is mandatory.
	implicitTLS bool
	tlsConfig   *tls.Config
}

// effectiveTLSConfig returns the client TLS configuration used for one send.
// Without a caller config it returns a config that verifies the server against
// the system roots under the SMTP host name with a TLS 1.2 minimum. A caller
// config is cloned (never mutated); its empty ServerName defaults to the SMTP
// host and a MinVersion below TLS 1.2 is raised to TLS 1.2.
func (s *requireTLSSender) effectiveTLSConfig() *tls.Config {
	if s.tlsConfig == nil {
		return &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	}

	cfg := s.tlsConfig.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = s.host
	}
	if cfg.MinVersion < tls.VersionTLS12 {
		cfg.MinVersion = tls.VersionTLS12
	}
	return cfg
}

// DialAndSend connects to the SMTP server, enforces TLS, optionally
// authenticates, and sends the messages. It returns an error (without sending
// credentials or message data) if the connection cannot be encrypted and
// verified.
func (s *requireTLSSender) DialAndSend(msgs ...*gomail.Message) error {
	tlsCfg := s.effectiveTLSConfig()

	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	conn, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return errors.Wrap(err, "dial smtp server")
	}

	// Implicit TLS (SMTPS, conventionally port 465): wrap the raw connection in
	// TLS immediately; the handshake runs before the SMTP greeting is read.
	if s.implicitTLS {
		conn = tls.Client(conn, tlsCfg)
	}

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return errors.Wrap(err, "create smtp client")
	}
	defer func() { _ = c.Close() }()

	if !s.implicitTLS {
		// Require STARTTLS: if the server does not advertise it, refuse to send
		// rather than silently transmitting credentials/content in plaintext.
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.Errorf(
				"smtp server %q does not advertise STARTTLS; refusing to send over plaintext", s.host)
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return errors.Wrap(err, "start tls")
		}
	}

	if s.username != "" || s.password != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			// The connection is encrypted at this point, so PlainAuth is safe.
			if err := c.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
				return errors.Wrap(err, "smtp auth")
			}
		}
	}

	// Reuse gomail's message serialization over the connection whose TLS state
	// we control.
	if err := gomail.Send(smtpSender{c: c}, msgs...); err != nil {
		return errors.Wrap(err, "send email over tls")
	}

	return nil
}

// smtpSender adapts a *smtp.Client to gomail's Sender interface so gomail's
// message serialization can be reused on a TLS connection we established.
type smtpSender struct{ c *smtp.Client }

// Send transmits a single message to the given recipients.
func (s smtpSender) Send(from string, to []string, msg io.WriterTo) error {
	if err := s.c.Mail(from); err != nil {
		return errors.Wrap(err, "MAIL FROM")
	}
	for _, addr := range to {
		if err := s.c.Rcpt(addr); err != nil {
			return errors.Wrapf(err, "RCPT TO %q", addr)
		}
	}

	w, err := s.c.Data()
	if err != nil {
		return errors.Wrap(err, "DATA")
	}
	if _, err := msg.WriteTo(w); err != nil {
		_ = w.Close()
		return errors.Wrap(err, "write message body")
	}

	return errors.Wrap(w.Close(), "close data writer")
}

// Send send email
func (m *MailT) Send(frAddr, toAddr, frName, toName, subject, content string, optfs ...SendOption) (err error) {
	// Validate all header fields against CRLF injection
	for field, value := range map[string]string{
		"frAddr":  frAddr,
		"toAddr":  toAddr,
		"frName":  frName,
		"toName":  toName,
		"subject": subject,
	} {
		if err := validateHeaderValue(field, value); err != nil {
			return err
		}
	}

	opt := new(mailSendOpt).fillDefault().applyOpts(optfs)
	log.Shared.Info("send email", zap.String("toName", toName))
	s := gomail.NewMessage()
	s.SetAddressHeader("From", frAddr, frName)
	s.SetAddressHeader("To", toAddr, toName)
	s.SetHeader("Subject", subject)
	s.SetBody("text/plain", content)

	dialer := opt.dialerFact(m.host, m.port, m.username, m.password)
	if err := dialer.DialAndSend(s); err != nil {
		return errors.Wrap(err, "try to send email got error")
	}

	return nil
}
