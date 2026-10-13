package email

import (
	"crypto/tls"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestSMTPCompatibilityNegotiation delivers only over TLS using mechanisms accepted by real legacy relays.
func TestSMTPCompatibilityNegotiation(t *testing.T) {
	for _, tc := range []struct{ advertised, selected string }{
		{"LOGIN", "LOGIN"}, {"CRAM-MD5", "CRAM-MD5"},
		{"PLAIN LOGIN", "PLAIN"}, {"PLAIN CRAM-MD5 LOGIN", "CRAM-MD5"},
	} {
		t.Run(tc.advertised, func(t *testing.T) {
			serverTLS, roots := newSMTPTestPKI(t)
			stub := startSMTPStub(t, smtpStubConfig{advertiseSTARTTLS: true,
				authMechanisms: tc.advertised, requireAuthMechanism: tc.selected, serverTLS: serverTLS})
			host, port := stub.hostPort(t)
			mail := NewMail(host, port)
			mail.Login(syntheticSMTPUser, syntheticSMTPPassword)
			require.NoError(t, sendMarkerMail(mail, WithEmailRequireTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})))
			commands := stub.commands(t)
			require.Contains(t, commandVerbs(commands), "AUTH")
			requireEncryptedBeforeSensitive(t, commands)
		})
	}
}
