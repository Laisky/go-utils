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

func TestMaskUsername(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		username string
		want     string
	}{
		{"empty", "", ""},
		{"email", "alice@example.com", "***@example.com"},
		{"email with dots", "first.last@sub.example.com", "***@sub.example.com"},
		{"plain", "alice", "***"},
		{"leading at", "@example.com", "***"},
		{"trailing at", "alice@", "***"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maskUsername(tc.username)
			require.Equal(t, tc.want, got)
			// The raw local part (account holder identifier) must never
			// survive masking for non-empty inputs.
			if tc.username != "" {
				require.NotEqual(t, tc.username, got)
			}
		})
	}
}

func TestLoginConfiguresAuth(t *testing.T) {
	t.Parallel()

	m := NewMail("smtp.example.com", 587)
	m.Login("alice@example.com", "s3cret")

	// Login must still store the raw credentials so Send can authenticate,
	// even though the logged value is masked.
	require.Equal(t, "alice@example.com", m.username)
	require.Equal(t, "s3cret", m.password)
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
