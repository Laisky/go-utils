// Package email simple email sender
package email

import (
	"github.com/Laisky/errors"
	zap "github.com/Laisky/zap"
	gomail "gopkg.in/gomail.v2"

	"github.com/Laisky/go-utils/v4/log"
)

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
	log.Shared.Debug("login", zap.String("username", username))
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
}

func (o *mailSendOpt) fillDefault() *mailSendOpt {
	o.dialerFact = func(host string, port int, username, passwd string) Sender {
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

// Send send email
func (m *MailT) Send(frAddr, toAddr, frName, toName, subject, content string, optfs ...SendOption) (err error) {
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
