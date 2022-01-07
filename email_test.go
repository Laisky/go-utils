package utils

import (
	"errors"
	"testing"

	"github.com/Laisky/go-utils/mocks"
	"github.com/Laisky/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func ExampleMail() {
	sender := NewMail("smtp_host", 53)
	if err := sender.Send(
		"fromAddr",
		"toAddr",
		"frName",
		"toName",
		"Title",
		"Content",
	); err != nil {
		Logger.Error("try to send email got error", zap.Error(err))
	}
}

func TestNewMail(t *testing.T) {
	m := NewMail("yo", 123)
	m.Login("username", "password")

	t.Run("ok", func(t *testing.T) {
		dialer := new(mocks.EmailDialer)
		dialer.On("DialAndSend", mock.Anything).Return(nil)
		err := m.Send(
			"from@email.com",
			"to@email.com",
			"fromName",
			"toName",
			"subject",
			m.BuildMessage("content"),
			WithMailSendDialer(func(host string, port int, username, passwd string) EmailDialer {
				return dialer
			}),
		)
		require.NoError(t, err)
	})

	t.Run("err", func(t *testing.T) {
		errWant := errors.New("yaho")
		dialer := new(mocks.EmailDialer)
		dialer.On("DialAndSend", mock.Anything).Return(errWant)
		err := m.Send(
			"from@email.com",
			"to@email.com",
			"fromName",
			"toName",
			"subject",
			m.BuildMessage("content"),
			WithMailSendDialer(func(host string, port int, username, passwd string) EmailDialer {
				return dialer
			}),
		)
		require.True(t, errors.Is(err, errWant))
	})
}
