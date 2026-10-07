package email

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// smtpStubDeadline bounds every loopback SMTP stub session so a stuck
	// client or server can never hang the test binary.
	smtpStubDeadline = 5 * time.Second
	// smtpStubMaxDataLines bounds how many DATA lines the stub accepts so a
	// misbehaving client cannot make the stub buffer unbounded input.
	smtpStubMaxDataLines = 1000
)

// smtpStubConfig configures the behavior of a disposable loopback SMTP stub.
// The stub never relays or delivers mail; it only records the command verbs
// it receives so tests can assert what a client sent and whether it was
// encrypted at that point.
type smtpStubConfig struct {
	// advertiseSTARTTLS makes the plaintext EHLO response list STARTTLS.
	advertiseSTARTTLS bool
	// refuseSTARTTLS makes the stub answer STARTTLS with a 454 failure,
	// simulating an on-path attacker or broken relay that blocks the upgrade.
	refuseSTARTTLS bool
	// implicitTLS wraps the accepted connection in TLS before the greeting,
	// mimicking an SMTPS (port 465 style) listener.
	implicitTLS bool
	// authMechanisms, when non-empty, is advertised as "AUTH <mechanisms>".
	authMechanisms string
	// serverTLS is the server-side TLS configuration used for STARTTLS or
	// implicit TLS.
	serverTLS *tls.Config
}

// smtpStubCommand records one SMTP command verb and whether the session was
// already protected by TLS when the stub received it.
type smtpStubCommand struct {
	verb string
	tls  bool
}

// smtpStub is a single-connection loopback SMTP server used by the transport
// security regression tests.
type smtpStub struct {
	ln   net.Listener
	cfg  smtpStubConfig
	done chan struct{}

	mu   sync.Mutex
	cmds []smtpStubCommand
}

// startSMTPStub starts a loopback SMTP stub described by cfg. It accepts a
// single connection, applies a strict session deadline, and is closed
// automatically when the test ends. It returns the running stub.
func startSMTPStub(t *testing.T, cfg smtpStubConfig) *smtpStub {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	stub := &smtpStub{ln: ln, cfg: cfg, done: make(chan struct{})}
	t.Cleanup(func() { _ = ln.Close() })

	go stub.serve()
	return stub
}

// hostPort returns the stub's loopback host and numeric port so tests can
// configure an SMTP client against it.
func (s *smtpStub) hostPort(t *testing.T) (string, int) {
	t.Helper()

	host, portStr, err := net.SplitHostPort(s.ln.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return host, port
}

// commands waits (bounded) for the stub session to finish and returns a copy
// of every command verb the stub received, in order.
func (s *smtpStub) commands(t *testing.T) []smtpStubCommand {
	t.Helper()

	select {
	case <-s.done:
	case <-time.After(smtpStubDeadline + time.Second):
		t.Fatal("smtp stub session did not finish in time")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]smtpStubCommand(nil), s.cmds...)
}

// record appends one received command verb together with the session's
// current TLS state.
func (s *smtpStub) record(verb string, inTLS bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cmds = append(s.cmds, smtpStubCommand{verb: verb, tls: inTLS})
}

// serve accepts exactly one connection and runs a minimal SMTP dialogue on
// it. Any protocol, deadline, or TLS failure simply ends the session.
func (s *smtpStub) serve() {
	defer close(s.done)

	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	if err = conn.SetDeadline(time.Now().UTC().Add(smtpStubDeadline)); err != nil {
		return
	}

	inTLS := false
	if s.cfg.implicitTLS {
		tlsConn := tls.Server(conn, s.cfg.serverTLS)
		if err = tlsConn.Handshake(); err != nil {
			return
		}
		conn, inTLS = tlsConn, true
	}

	br := bufio.NewReader(conn)
	if !writeSMTPLine(conn, "220 stub.example.invalid ESMTP ready") {
		return
	}

	for {
		line, rerr := br.ReadString('\n')
		if rerr != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			if !writeSMTPLine(conn, "500 empty command") {
				return
			}
			continue
		}
		verb := strings.ToUpper(fields[0])
		s.record(verb, inTLS)

		switch verb {
		case "EHLO", "HELO":
			if !writeSMTPLines(conn, s.ehloLines(inTLS)) {
				return
			}
		case "STARTTLS":
			if s.cfg.refuseSTARTTLS || inTLS || s.cfg.serverTLS == nil {
				if !writeSMTPLine(conn, "454 4.7.0 TLS not available") {
					return
				}
				continue
			}
			if !writeSMTPLine(conn, "220 2.0.0 ready to start TLS") {
				return
			}
			tlsConn := tls.Server(conn, s.cfg.serverTLS)
			if err = tlsConn.Handshake(); err != nil {
				return
			}
			conn, inTLS = tlsConn, true
			br = bufio.NewReader(conn)
		case "AUTH":
			if !s.handleAuth(conn, br, fields) {
				return
			}
		case "DATA":
			if !s.handleData(conn, br) {
				return
			}
		case "QUIT":
			_ = writeSMTPLine(conn, "221 2.0.0 bye")
			return
		default:
			if !writeSMTPLine(conn, "250 2.0.0 ok") {
				return
			}
		}
	}
}

// ehloLines builds the multi-line EHLO response for the current TLS state.
// STARTTLS is only advertised before the session is encrypted.
func (s *smtpStub) ehloLines(inTLS bool) []string {
	caps := []string{"stub.example.invalid"}
	if s.cfg.advertiseSTARTTLS && !inTLS {
		caps = append(caps, "STARTTLS")
	}
	if s.cfg.authMechanisms != "" {
		caps = append(caps, "AUTH "+s.cfg.authMechanisms)
	}
	caps = append(caps, "SIZE 1024")

	lines := make([]string, 0, len(caps))
	for i, c := range caps {
		sep := "-"
		if i == len(caps)-1 {
			sep = " "
		}
		lines = append(lines, "250"+sep+c)
	}
	return lines
}

// handleAuth runs a minimal AUTH PLAIN or AUTH LOGIN exchange that accepts
// any synthetic credentials without inspecting or retaining them. It returns
// false when the session should end.
func (s *smtpStub) handleAuth(conn net.Conn, br *bufio.Reader, fields []string) bool {
	mech := ""
	if len(fields) > 1 {
		mech = strings.ToUpper(fields[1])
	}

	switch {
	case mech == "LOGIN":
		// Prompt for username and password ("Username:" / "Password:" in base64).
		for _, prompt := range []string{"334 VXNlcm5hbWU6", "334 UGFzc3dvcmQ6"} {
			if !writeSMTPLine(conn, prompt) {
				return false
			}
			if _, err := br.ReadString('\n'); err != nil {
				return false
			}
		}
	case mech == "PLAIN" && len(fields) < 3:
		if !writeSMTPLine(conn, "334 ") {
			return false
		}
		if _, err := br.ReadString('\n'); err != nil {
			return false
		}
	case mech != "PLAIN":
		return writeSMTPLine(conn, "504 5.5.4 unsupported mechanism")
	}

	return writeSMTPLine(conn, "235 2.7.0 authentication succeeded")
}

// handleData accepts a bounded DATA payload terminated by a lone "." line and
// discards it. It returns false when the session should end.
func (s *smtpStub) handleData(conn net.Conn, br *bufio.Reader) bool {
	if !writeSMTPLine(conn, "354 end data with <CR><LF>.<CR><LF>") {
		return false
	}
	for range smtpStubMaxDataLines {
		line, err := br.ReadString('\n')
		if err != nil {
			return false
		}
		if strings.TrimRight(line, "\r\n") == "." {
			return writeSMTPLine(conn, "250 2.0.0 queued")
		}
	}
	return false
}

// writeSMTPLine writes one CRLF-terminated SMTP reply line and reports
// whether the write succeeded.
func writeSMTPLine(w io.Writer, line string) bool {
	_, err := io.WriteString(w, line+"\r\n")
	return err == nil
}

// writeSMTPLines writes several CRLF-terminated SMTP reply lines and reports
// whether every write succeeded.
func writeSMTPLines(w io.Writer, lines []string) bool {
	for _, line := range lines {
		if !writeSMTPLine(w, line) {
			return false
		}
	}
	return true
}

// commandVerbs extracts only the verbs from recorded stub commands, which
// keeps assertion failure messages compact.
func commandVerbs(cmds []smtpStubCommand) []string {
	verbs := make([]string, 0, len(cmds))
	for _, c := range cmds {
		verbs = append(verbs, c.verb)
	}
	return verbs
}

// requireNoSensitiveCommands asserts that the stub never received AUTH or any
// envelope/message command (MAIL, RCPT, DATA), i.e. that the client aborted
// before transmitting credentials or message content.
func requireNoSensitiveCommands(t *testing.T, cmds []smtpStubCommand) {
	t.Helper()

	for _, c := range cmds {
		switch c.verb {
		case "AUTH", "MAIL", "RCPT", "DATA":
			t.Fatalf("client sent %s before rejecting the transport; commands: %v", c.verb, commandVerbs(cmds))
		}
	}
}

// newSMTPTestPKI generates an in-memory ECDSA test CA and a leaf certificate
// valid for 127.0.0.1 and localhost. It returns the server-side TLS config
// presenting the leaf and a root pool trusting only the test CA.
func newSMTPTestPKI(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()

	now := time.Now().UTC()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "go-utils email test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "stub.example.invalid"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}},
		MinVersion:   tls.VersionTLS12,
	}
	return serverTLS, roots
}
