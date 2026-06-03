package email

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	gomail "gopkg.in/gomail.v2"
)

// TestWithEmailRequireTLS_SelectsTLSSender verifies that WithEmailRequireTLS
// switches the default dialer factory to the TLS-enforcing sender, while the
// default (no option) keeps using gomail's dialer.
func TestWithEmailRequireTLS_SelectsTLSSender(t *testing.T) {
	t.Parallel()

	t.Run("require tls selects requireTLSSender", func(t *testing.T) {
		opt := new(mailSendOpt).fillDefault().applyOpts([]SendOption{WithEmailRequireTLS()})
		s := opt.dialerFact("smtp.example.com", 587, "u", "p")
		ts, ok := s.(*requireTLSSender)
		require.True(t, ok, "expected *requireTLSSender, got %T", s)
		require.Equal(t, "smtp.example.com", ts.host)
		require.Equal(t, 587, ts.port)
	})

	t.Run("default selects gomail dialer", func(t *testing.T) {
		opt := new(mailSendOpt).fillDefault().applyOpts(nil)
		s := opt.dialerFact("smtp.example.com", 587, "u", "p")
		_, ok := s.(*gomail.Dialer)
		require.True(t, ok, "expected *gomail.Dialer, got %T", s)
	})

	t.Run("custom tls config is stored", func(t *testing.T) {
		cfg := &tls.Config{ServerName: "custom", MinVersion: tls.VersionTLS13}
		opt := new(mailSendOpt).fillDefault().applyOpts([]SendOption{WithEmailRequireTLS(cfg)})
		s := opt.dialerFact("smtp.example.com", 465, "u", "p")
		ts, ok := s.(*requireTLSSender)
		require.True(t, ok, "expected *requireTLSSender, got %T", s)
		require.Same(t, cfg, ts.tlsConfig)
	})
}

// TestRequireTLSSender_RefusesPlaintextWhenNoSTARTTLS is the security
// regression test: against a server that does NOT advertise STARTTLS (as a
// STARTTLS-stripping attacker would force), the sender must return an error and
// must NOT fall back to transmitting credentials/content in plaintext.
func TestRequireTLSSender_RefusesPlaintextWhenNoSTARTTLS(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	// Minimal fake SMTP server that greets and answers EHLO WITHOUT advertising
	// STARTTLS. It records whether the client ever attempted MAIL FROM (which
	// would indicate an unsafe plaintext send).
	mailAttempted := make(chan struct{}, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		br := bufio.NewReader(conn)
		_, _ = io.WriteString(conn, "220 fake ESMTP ready\r\n")
		for {
			line, rerr := br.ReadString('\n')
			if rerr != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				// Advertise a capability but deliberately omit STARTTLS.
				_, _ = io.WriteString(conn, "250-fake.example.com\r\n250 SIZE 35882577\r\n")
			case strings.HasPrefix(cmd, "MAIL"):
				select {
				case mailAttempted <- struct{}{}:
				default:
				}
				_, _ = io.WriteString(conn, "250 ok\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "250 ok\r\n")
			}
		}
	}()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	sender := &requireTLSSender{
		host:     host,
		port:     port,
		username: "alice@example.com",
		password: "s3cret",
	}

	msg := gomail.NewMessage()
	msg.SetHeader("From", "from@example.com")
	msg.SetHeader("To", "to@example.com")
	msg.SetHeader("Subject", "hi")
	msg.SetBody("text/plain", "body")

	err = sender.DialAndSend(msg)
	require.Error(t, err, "sending must fail when STARTTLS is unavailable")
	require.Contains(t, err.Error(), "STARTTLS")

	// The sender must have aborted before transmitting the envelope.
	select {
	case <-mailAttempted:
		t.Fatal("sender transmitted MAIL FROM over a plaintext connection")
	default:
	}
}
