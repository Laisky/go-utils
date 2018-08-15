package utils

import (
	"github.com/pkg/errors"
	zap "go.uber.org/zap"
	gomail "gopkg.in/gomail.v2"
)

type Mail struct {
	host               string
	port               int
	username, password string
}

func NewMail(host string, port int) *Mail {
	Logger.Debug("try to send mail", zap.String("host", host), zap.Int("port", port))
	return &Mail{
		host: host,
		port: port,
	}
}

func (m *Mail) Login(username, password string) {
	Logger.Debug("login", zap.String("username", username))
	m.username = username
	m.password = password
}

func (m *Mail) BuildMessage(msg string) string {
	return msg
}

func (m *Mail) Send(fr, to, frName, toName, subject, content string) (err error) {
	Logger.Info("send email", zap.String("toName", toName))
	s := gomail.NewMessage()
	s.SetAddressHeader("From", fr, frName)
	s.SetAddressHeader("To", to, toName)
	s.SetHeader("Subject", subject)
	s.SetBody("text/plain", content)

	d := gomail.NewPlainDialer(m.host, m.port, m.username, m.password)

	if err := d.DialAndSend(s); err != nil {
		return errors.Wrap(err, "try to send email got error")
	}

	return nil
}
