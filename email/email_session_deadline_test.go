package email

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gomail "gopkg.in/gomail.v2"
)

// TestRequireTLSSenderBoundsStalledSession verifies that the strict SMTP sender
// gives up on a server that accepts the TCP connection but never speaks SMTP,
// instead of blocking the caller forever.
func TestRequireTLSSenderBoundsStalledSession(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()
	defer func() {
		select {
		case conn := <-accepted:
			_ = conn.Close()
		default:
		}
	}()

	original := smtpSessionTimeout
	smtpSessionTimeout = 300 * time.Millisecond
	defer func() { smtpSessionTimeout = original }()

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	sender := &requireTLSSender{host: host, port: port}
	done := make(chan error, 1)
	go func() {
		done <- sender.DialAndSend(gomail.NewMessage())
	}()

	select {
	case sendErr := <-done:
		require.Error(t, sendErr)
	case <-time.After(5 * time.Second):
		t.Fatal("DialAndSend blocked on a stalled SMTP server")
	}
}
