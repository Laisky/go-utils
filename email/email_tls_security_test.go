package email

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
	gomail "gopkg.in/gomail.v2"
)

const (
	// syntheticSMTPUser is a fake SMTP login used only against loopback stubs.
	syntheticSMTPUser = "synthetic-user@example.invalid"
	// syntheticSMTPPassword is a fake SMTP password used only against loopback stubs.
	syntheticSMTPPassword = "SYNTHETIC_PASSWORD"
)

// sendMarkerMail sends one tiny marker-only message with synthetic addresses
// through m using the supplied options and returns Send's error.
func sendMarkerMail(m *MailT, optfs ...SendOption) error {
	return m.Send(
		"sender@example.invalid",
		"rcpt@example.invalid",
		"Sender",
		"Recipient",
		"marker",
		"marker",
		optfs...,
	)
}

// TestDefaultSendOptionSelectsRequireTLSSender verifies that a zero-option
// send selects the TLS-enforcing sender instead of gomail's opportunistic
// STARTTLS dialer. It is the default-selection regression for issue #58.
func TestDefaultSendOptionSelectsRequireTLSSender(t *testing.T) {
	t.Parallel()

	opt := new(mailSendOpt).fillDefault().applyOpts(nil)
	s := opt.dialerFact("smtp.example.invalid", 587, syntheticSMTPUser, syntheticSMTPPassword)
	ts, ok := s.(*requireTLSSender)
	require.True(t, ok, "zero-option send must require TLS, got %T", s)
	require.Equal(t, "smtp.example.invalid", ts.host)
	require.Equal(t, 587, ts.port)
}

// TestZeroOptionSendRefusesServerWithoutSTARTTLS verifies that a zero-option
// MailT.Send against a server that does not advertise STARTTLS (the state a
// STARTTLS-stripping attacker forces) fails before AUTH, MAIL, RCPT, or DATA
// is transmitted. It is the end-to-end regression for issue #58.
func TestZeroOptionSendRefusesServerWithoutSTARTTLS(t *testing.T) {
	t.Parallel()

	stub := startSMTPStub(t, smtpStubConfig{authMechanisms: "LOGIN"})
	host, port := stub.hostPort(t)

	m := NewMail(host, port)
	m.Login(syntheticSMTPUser, syntheticSMTPPassword)

	err := sendMarkerMail(m)
	require.Error(t, err, "zero-option send must not deliver over plaintext")
	require.Contains(t, err.Error(), "STARTTLS")

	requireNoSensitiveCommands(t, stub.commands(t))
}

// requireEncryptedBeforeSensitive asserts that every AUTH, MAIL, RCPT, and DATA
// command the stub saw arrived over TLS and that each of MAIL, RCPT, and DATA
// was actually received, proving the message was delivered only after the
// TLS handshake completed.
func requireEncryptedBeforeSensitive(t *testing.T, cmds []smtpStubCommand) {
	t.Helper()

	seen := map[string]bool{}
	for _, c := range cmds {
		switch c.verb {
		case "AUTH", "MAIL", "RCPT", "DATA":
			require.True(t, c.tls, "%s was sent before TLS; commands: %v", c.verb, commandVerbs(cmds))
			seen[c.verb] = true
		}
	}
	for _, verb := range []string{"MAIL", "RCPT", "DATA"} {
		require.True(t, seen[verb], "expected %s to be delivered; commands: %v", verb, commandVerbs(cmds))
	}
}

// TestDefaultSendSelectsImplicitTLSOnSMTPSPort verifies that the zero-option
// default uses implicit TLS on port 465 and mandatory STARTTLS elsewhere, and
// that conflicting options fail closed. It is a selection regression for
// issue #58.
func TestDefaultSendSelectsImplicitTLSOnSMTPSPort(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		port         int
		opts         []SendOption
		wantImplicit bool
	}{
		{name: "default 465 implicit tls", port: 465, wantImplicit: true},
		{name: "default 587 starttls", port: 587},
		{name: "default 25 starttls", port: 25},
		{name: "require tls wins over insecure", port: 587,
			opts: []SendOption{WithEmailRequireTLS(), WithEmailInsecureAllowPlaintext()}},
		{name: "require tls wins regardless of order", port: 465, wantImplicit: true,
			opts: []SendOption{WithEmailInsecureAllowPlaintext(), WithEmailRequireTLS()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := new(mailSendOpt).fillDefault().applyOpts(tc.opts)
			s := opt.dialerFact("smtp.example.invalid", tc.port, syntheticSMTPUser, syntheticSMTPPassword)
			ts, ok := s.(*requireTLSSender)
			require.True(t, ok, "expected *requireTLSSender, got %T", s)
			require.Equal(t, tc.wantImplicit, ts.implicitTLS)
		})
	}

	t.Run("insecure opt-out keeps gomail ssl on 465", func(t *testing.T) {
		t.Parallel()

		opt := new(mailSendOpt).fillDefault().applyOpts([]SendOption{WithEmailInsecureAllowPlaintext()})
		d, ok := opt.dialerFact("smtp.example.invalid", 465, "", "").(*gomail.Dialer)
		require.True(t, ok)
		require.True(t, d.SSL)
	})

	t.Run("custom dialer takes precedence", func(t *testing.T) {
		t.Parallel()

		custom := &requireTLSSender{host: "custom.example.invalid"}
		opt := new(mailSendOpt).fillDefault().applyOpts([]SendOption{
			WithEmailRequireTLS(),
			WithMailSendDialer(func(string, int, string, string) Sender { return custom }),
		})
		require.Same(t, custom, opt.dialerFact("smtp.example.invalid", 587, "", ""))
	})
}

// TestRequireTLSSenderEffectiveTLSConfig verifies how caller TLS configs
// interact with the secure default: they are cloned, an empty ServerName
// defaults to the SMTP host, MinVersion is floored at TLS 1.2, and stricter
// caller choices are preserved. It is a regression for issue #58.
func TestRequireTLSSenderEffectiveTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("default config", func(t *testing.T) {
		t.Parallel()

		cfg := (&requireTLSSender{host: "smtp.example.invalid"}).effectiveTLSConfig()
		require.Equal(t, "smtp.example.invalid", cfg.ServerName)
		require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		require.False(t, cfg.InsecureSkipVerify)
	})

	t.Run("caller config is hardened on a clone", func(t *testing.T) {
		t.Parallel()

		caller := &tls.Config{MinVersion: tls.VersionTLS10}
		cfg := (&requireTLSSender{host: "smtp.example.invalid", tlsConfig: caller}).effectiveTLSConfig()
		require.NotSame(t, caller, cfg)
		require.Equal(t, "smtp.example.invalid", cfg.ServerName)
		require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		require.Empty(t, caller.ServerName, "caller config must not be mutated")
		require.Equal(t, uint16(tls.VersionTLS10), caller.MinVersion, "caller config must not be mutated")
	})

	t.Run("stricter caller choices are preserved", func(t *testing.T) {
		t.Parallel()

		caller := &tls.Config{ServerName: "relay.example.invalid", MinVersion: tls.VersionTLS13}
		cfg := (&requireTLSSender{host: "smtp.example.invalid", tlsConfig: caller}).effectiveTLSConfig()
		require.Equal(t, "relay.example.invalid", cfg.ServerName)
		require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	})
}

// TestRequireTLSSendSucceedsAfterSTARTTLS verifies that a send with a trusted
// test CA upgrades with STARTTLS and only then authenticates and transmits the
// envelope and message. It is a compatible-path regression for issue #58.
func TestRequireTLSSendSucceedsAfterSTARTTLS(t *testing.T) {
	t.Parallel()

	serverTLS, roots := newSMTPTestPKI(t)
	stub := startSMTPStub(t, smtpStubConfig{
		advertiseSTARTTLS: true,
		authMechanisms:    "PLAIN",
		serverTLS:         serverTLS,
	})
	host, port := stub.hostPort(t)

	m := NewMail(host, port)
	m.Login(syntheticSMTPUser, syntheticSMTPPassword)
	require.NoError(t, sendMarkerMail(m, WithEmailRequireTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})))

	cmds := stub.commands(t)
	require.Contains(t, commandVerbs(cmds), "STARTTLS")
	require.Contains(t, commandVerbs(cmds), "AUTH")
	requireEncryptedBeforeSensitive(t, cmds)
}

// TestDefaultSendRejectsUntrustedCertificate verifies that the zero-option
// default verifies the STARTTLS certificate against the system roots and
// aborts before AUTH or message data when the chain is untrusted. It is a
// certificate-failure regression for issue #58.
func TestDefaultSendRejectsUntrustedCertificate(t *testing.T) {
	t.Parallel()

	serverTLS, _ := newSMTPTestPKI(t)
	stub := startSMTPStub(t, smtpStubConfig{
		advertiseSTARTTLS: true,
		authMechanisms:    "PLAIN LOGIN",
		serverTLS:         serverTLS,
	})
	host, port := stub.hostPort(t)

	m := NewMail(host, port)
	m.Login(syntheticSMTPUser, syntheticSMTPPassword)
	err := sendMarkerMail(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start tls")
	require.Contains(t, err.Error(), "unknown authority")

	requireNoSensitiveCommands(t, stub.commands(t))
}

// TestRequireTLSSendRejectsHostnameMismatch verifies that a certificate from a
// trusted CA is still rejected when it does not match the expected server
// name, before AUTH or message data. It is a hostname-verification regression
// for issue #58.
func TestRequireTLSSendRejectsHostnameMismatch(t *testing.T) {
	t.Parallel()

	serverTLS, roots := newSMTPTestPKI(t)
	stub := startSMTPStub(t, smtpStubConfig{
		advertiseSTARTTLS: true,
		authMechanisms:    "PLAIN",
		serverTLS:         serverTLS,
	})
	host, port := stub.hostPort(t)

	m := NewMail(host, port)
	m.Login(syntheticSMTPUser, syntheticSMTPPassword)
	err := sendMarkerMail(m, WithEmailRequireTLS(&tls.Config{
		RootCAs:    roots,
		ServerName: "mismatch.example.invalid",
		MinVersion: tls.VersionTLS12,
	}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "start tls")
	require.Contains(t, err.Error(), "mismatch.example.invalid")

	requireNoSensitiveCommands(t, stub.commands(t))
}

// TestDefaultSendRejectsRefusedSTARTTLS verifies that a STARTTLS capability
// that is advertised but then refused (a downgrade attempt) aborts the send
// before AUTH or message data instead of continuing in plaintext. It is a
// downgrade regression for issue #58.
func TestDefaultSendRejectsRefusedSTARTTLS(t *testing.T) {
	t.Parallel()

	stub := startSMTPStub(t, smtpStubConfig{
		advertiseSTARTTLS: true,
		refuseSTARTTLS:    true,
		authMechanisms:    "PLAIN LOGIN",
	})
	host, port := stub.hostPort(t)

	m := NewMail(host, port)
	m.Login(syntheticSMTPUser, syntheticSMTPPassword)
	err := sendMarkerMail(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start tls")

	requireNoSensitiveCommands(t, stub.commands(t))
}

// TestRequireTLSSenderImplicitTLS verifies the implicit-TLS (port 465 style)
// path on an unprivileged loopback port: the handshake completes before the
// SMTP greeting, every command is encrypted, and an untrusted certificate is
// rejected before any command is sent. It is an implicit-TLS regression for
// issue #58.
func TestRequireTLSSenderImplicitTLS(t *testing.T) {
	t.Parallel()

	msg := gomail.NewMessage()
	msg.SetHeader("From", "sender@example.invalid")
	msg.SetHeader("To", "rcpt@example.invalid")
	msg.SetHeader("Subject", "marker")
	msg.SetBody("text/plain", "marker")

	t.Run("trusted", func(t *testing.T) {
		t.Parallel()

		serverTLS, roots := newSMTPTestPKI(t)
		stub := startSMTPStub(t, smtpStubConfig{implicitTLS: true, authMechanisms: "PLAIN", serverTLS: serverTLS})
		host, port := stub.hostPort(t)

		sender := &requireTLSSender{
			host: host, port: port, implicitTLS: true,
			username: syntheticSMTPUser, password: syntheticSMTPPassword,
			tlsConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		}
		require.NoError(t, sender.DialAndSend(msg))

		cmds := stub.commands(t)
		require.NotContains(t, commandVerbs(cmds), "STARTTLS")
		for _, c := range cmds {
			require.True(t, c.tls, "%s was sent before TLS", c.verb)
		}
		requireEncryptedBeforeSensitive(t, cmds)
	})

	t.Run("untrusted", func(t *testing.T) {
		t.Parallel()

		serverTLS, _ := newSMTPTestPKI(t)
		stub := startSMTPStub(t, smtpStubConfig{implicitTLS: true, authMechanisms: "PLAIN", serverTLS: serverTLS})
		host, port := stub.hostPort(t)

		sender := &requireTLSSender{
			host: host, port: port, implicitTLS: true,
			username: syntheticSMTPUser, password: syntheticSMTPPassword,
		}
		require.Error(t, sender.DialAndSend(msg))
		require.Empty(t, stub.commands(t), "no SMTP command may precede a failed implicit TLS handshake")
	})
}

// TestInsecureAllowPlaintextOptOut verifies that the explicit insecure opt-out
// restores the legacy opportunistic behavior so plaintext-only relays keep
// working when the caller deliberately chooses it. It is the compatibility
// regression for issue #58.
func TestInsecureAllowPlaintextOptOut(t *testing.T) {
	t.Parallel()

	stub := startSMTPStub(t, smtpStubConfig{})
	host, port := stub.hostPort(t)

	m := NewMail(host, port)
	require.NoError(t, sendMarkerMail(m, WithEmailInsecureAllowPlaintext()))

	verbs := commandVerbs(stub.commands(t))
	require.NotContains(t, verbs, "STARTTLS")
	for _, verb := range []string{"MAIL", "RCPT", "DATA"} {
		require.Contains(t, verbs, verb)
	}
}
