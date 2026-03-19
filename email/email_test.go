package email

import (
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/go-utils/v6/mocks"
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
		log.Shared.Error("try to send email got error", zap.Error(err))
	}
}

func TestEmailHeaderInjection(t *testing.T) {
	t.Parallel()

	m := NewMail("yo", 123)
	m.Login("username", "password")

	dialer := new(mocks.EmailDialer)
	dialer.On("DialAndSend", mock.Anything).Return(nil)
	dialerOpt := WithMailSendDialer(func(host string, port int, username, passwd string) Sender {
		return dialer
	})

	// CRLF in toAddr
	err := m.Send("from@a.com", "to@a.com\r\nBCC: evil@a.com", "fr", "to", "subj", "body", dialerOpt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "header injection")

	// Newline in subject
	err = m.Send("from@a.com", "to@a.com", "fr", "to", "subj\nBCC: evil@a.com", "body", dialerOpt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "header injection")

	// CR in frName
	err = m.Send("from@a.com", "to@a.com", "fr\rname", "to", "subj", "body", dialerOpt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "header injection")

	// Clean input should succeed
	err = m.Send("from@a.com", "to@a.com", "fr", "to", "subj", "body", dialerOpt)
	require.NoError(t, err)
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
			"content",
			WithMailSendDialer(func(host string, port int, username, passwd string) Sender {
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
			"content",
			WithMailSendDialer(func(host string, port int, username, passwd string) Sender {
				return dialer
			}),
		)
		require.True(t, errors.Is(err, errWant))
	})
}
