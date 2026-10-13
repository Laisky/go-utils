package email

import (
	"crypto/tls"
	"net/smtp"
	"testing"

	"github.com/stretchr/testify/require"
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

// TestSMTPCompatibilityNoDowngrade rejects lookalike capabilities and refuses LOGIN credential disclosure off TLS.
func TestSMTPCompatibilityNoDowngrade(t *testing.T) {
	for _, advertised := range []string{"XLOGIN", "XPLAIN", "SCRAM-SHA-256", ""} {
		auth, err := selectSMTPAuth(advertised, syntheticSMTPUser, syntheticSMTPPassword, "relay.invalid")
		require.Error(t, err)
		require.Nil(t, auth)
	}
	auth := &smtpLoginAuth{username: syntheticSMTPUser, password: syntheticSMTPPassword, host: "relay.invalid"}
	for _, server := range []*smtp.ServerInfo{nil, {Name: "relay.invalid"}, {TLS: true, Name: "other.invalid"}} {
		_, response, err := auth.Start(server)
		require.Error(t, err)
		require.Nil(t, response)
	}
	_, _, err := auth.Start(&smtp.ServerInfo{TLS: true, Name: "relay.invalid"})
	require.NoError(t, err)
	response, err := auth.Next([]byte("Password:"), true)
	require.Error(t, err)
	require.Nil(t, response)
	for _, prompt := range []string{"Username:", "Password:"} {
		response, err = auth.Next([]byte(prompt), true)
		require.NoError(t, err)
		require.NotEmpty(t, response)
	}
	response, err = auth.Next([]byte("Password:"), true)
	require.Error(t, err)
	require.Nil(t, response)
}
