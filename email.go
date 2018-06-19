package utils

import (
	"github.com/pkg/errors"
	gomail "gopkg.in/gomail.v2"
)

type Mail struct {
	host               string
	port               int
	username, password string
}

func NewMail(host string, port int) *Mail {
	Logger.Debugf("new mail for host %v, port %v", host, port)
	return &Mail{
		host: host,
		port: port,
	}
}

func (m *Mail) Login(username, password string) {
	Logger.Debugf("login for %v", username)
	m.username = username
	m.password = password
}

func (m *Mail) BuildMessage(msg string) string {
	return msg
}

func (m *Mail) Send(fr, to, frName, toName, subject, content string) (err error) {
	Logger.Infof("send email to %v", toName)
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
