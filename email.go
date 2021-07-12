package utils

import (
	zap "github.com/Laisky/zap"
	"github.com/pkg/errors"
	gomail "gopkg.in/gomail.v2"
)

// Mail easy way to send basic email
type Mail struct {
	host               string
	port               int
	username, password string
}

// NewMail create Mail with SMTP host and port
func NewMail(host string, port int) *Mail {
	Logger.Debug("try to send mail", zap.String("host", host), zap.Int("port", port))
	return &Mail{
		host: host,
		port: port,
	}
}

// Login login to SMTP server
func (m *Mail) Login(username, password string) {
	Logger.Debug("login", zap.String("username", username))
	m.username = username
	m.password = password
}

// BuildMessage implement
func (m *Mail) BuildMessage(msg string) string {
	return msg
}

// Send send email
func (m *Mail) Send(frAddr, toAddr, frName, toName, subject, content string) (err error) {
	Logger.Info("send email", zap.String("toName", toName))
	s := gomail.NewMessage()
	s.SetAddressHeader("From", frAddr, frName)
	s.SetAddressHeader("To", toAddr, toName)
	s.SetHeader("Subject", subject)
	s.SetBody("text/plain", content)

	d := gomail.NewDialer(m.host, m.port, m.username, m.password)

	if Settings.GetBool("dry") {
		Logger.Info("try to send email",
			zap.String("fromAddr", frAddr),
			zap.String("toAddr", toAddr),
			zap.String("frName", frName),
			zap.String("toName", toName),
			zap.String("subject", subject),
			zap.String("content", content),
		)
	} else {
		if err := d.DialAndSend(s); err != nil {
			return errors.Wrap(err, "try to send email got error")
		}
	}

	return nil
}
