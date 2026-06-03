// Package email simple email sender
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

// smtpDialTimeout bounds how long the TLS-enforcing sender waits to establish
// the TCP connection to the SMTP server.
const smtpDialTimeout = 30 * time.Second

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

type mailSendOpt struct {
	dialerFact func(host string, port int, username, passwd string) Sender
	requireTLS bool
	tlsConfig  *tls.Config
}

func (o *mailSendOpt) fillDefault() *mailSendOpt {
	o.dialerFact = func(host string, port int, username, passwd string) Sender {
		// Security: when TLS is required, use a sender that guarantees the
		// session is encrypted and refuses any plaintext fallback, instead of
		// gomail's default opportunistic-STARTTLS dialer (which can be downgraded
		// by a STARTTLS-stripping attacker).
		if o.requireTLS {
			return &requireTLSSender{
				host:      host,
				port:      port,
				username:  username,
				password:  passwd,
				tlsConfig: o.tlsConfig,
			}
		}

		return gomail.NewDialer(host, port, username, passwd)
	}

	return o
}

func (o *mailSendOpt) applyOpts(optfs []SendOption) *mailSendOpt {
	for _, optf := range optfs {
		optf(o)
	}
	return o
}

// SendOption is a function to set option for Mail.Send
type SendOption func(*mailSendOpt)

// WithMailSendDialer set gomail.Dialer
func WithMailSendDialer(dialerFact func(host string, port int, username, passwd string) Sender) SendOption {
	return func(opt *mailSendOpt) {
		opt.dialerFact = dialerFact
	}
}

// WithEmailRequireTLS enforces an encrypted SMTP connection and refuses to fall
// back to plaintext.
//
// Security: by default delivery uses gomail's opportunistic STARTTLS. A network
// attacker can defeat that by stripping the STARTTLS capability from the
// server's EHLO response, causing credentials and message content to be
// transmitted in cleartext (a STARTTLS-stripping downgrade attack). With this
// option set, Send requires encryption: for the conventional implicit-TLS port
// 465 it dials directly over TLS, and for any other port it requires the server
// to advertise STARTTLS and aborts WITHOUT sending if it does not.
//
// An optional *tls.Config may be supplied; when nil a config using the SMTP host
// as ServerName and a TLS 1.2 minimum is used.
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

// requireTLSSender is a Sender that guarantees the SMTP session is encrypted
// before any credentials or message data are transmitted, defending against
// STARTTLS-stripping downgrade attacks.
type requireTLSSender struct {
	host, username, password string
	port                     int
	tlsConfig                *tls.Config
}

// DialAndSend connects to the SMTP server, enforces TLS, optionally
// authenticates, and sends the messages. It returns an error (without sending)
// if the connection cannot be encrypted.
func (s *requireTLSSender) DialAndSend(msgs ...*gomail.Message) error {
	tlsCfg := s.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	}

	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	conn, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return errors.Wrap(err, "dial smtp server")
	}

	// Port 465 is the conventional implicit-TLS (SMTPS) port: wrap the raw
	// connection in TLS immediately.
	if s.port == 465 {
		conn = tls.Client(conn, tlsCfg)
	}

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return errors.Wrap(err, "create smtp client")
	}
	defer func() { _ = c.Close() }()

	if s.port != 465 {
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
