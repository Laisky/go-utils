package crypto

import (
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOidAsn2X509(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   asn1.ObjectIdentifier
		want    string
		wantErr bool
	}{
		{
			name:    "valid OID",
			input:   asn1.ObjectIdentifier{1, 2, 3, 4},
			want:    "1.2.3.4",
			wantErr: false,
		},
		{
			name:    "empty OID",
			input:   asn1.ObjectIdentifier{},
			want:    "",
			wantErr: false,
		},
		{
			name:    "negative value",
			input:   asn1.ObjectIdentifier{1, -2, 3},
			wantErr: true,
		},
		{
			name:    "long OID",
			input:   asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			want:    "1.2.3.4.5.6.7.8.9.10",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := OidAsn2X509(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err) // <-- 665
			require.Equal(t, tt.want, got.String())
		})
	}
}

func Test_OIDs(t *testing.T) {
	t.Parallel()

	t.Run("compare OIDs", func(t *testing.T) {
		t.Parallel()

		a1 := asn1.ObjectIdentifier{1, 2, 3}
		a2 := asn1.ObjectIdentifier{1, 2, 3}
		a3 := asn1.ObjectIdentifier{1, 2, 3, 4}
		require.Equal(t, a1, a2)
		require.NotEqual(t, a1, a3)
		require.NotEqual(t, a2, a3)
	})

	t.Run("valid policy OIDs", func(t *testing.T) {
		t.Parallel()

		// Using valid policy OIDs
		// As per RFC 5280, policy OIDs should start with 2.5.29.32
		policyOID1 := asn1.ObjectIdentifier{2, 5, 29, 32, 0}
		policyOID2 := asn1.ObjectIdentifier{2, 5, 29, 32, 1}

		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertPolicies(policyOID1, policyOID2),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		require.Contains(t, ca.PolicyIdentifiers, policyOID1)
		require.Contains(t, ca.PolicyIdentifiers, policyOID2)
		require.NotContains(t, ca.PolicyIdentifiers, asn1.ObjectIdentifier{2, 5, 29, 32, 2})
		oid1, err := OidAsn2X509(policyOID1)
		require.NoError(t, err)
		oid2, err := OidAsn2X509(policyOID2)
		require.NoError(t, err)
		require.Contains(t, ca.Policies, oid1)
		require.Contains(t, ca.Policies, oid2)
	})

	t.Run("OID prefix matching", func(t *testing.T) {
		t.Parallel()

		policyOID := asn1.ObjectIdentifier{2, 5, 29, 32, 0}
		prefix := asn1.ObjectIdentifier{2, 5, 29}

		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertPolicies(policyOID),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		require.True(t, OIDContains(ca.PolicyIdentifiers, policyOID))
		require.True(t, OIDContains(ca.PolicyIdentifiers, prefix, MatchPrefix()))
		require.False(t, OIDContains(ca.PolicyIdentifiers, asn1.ObjectIdentifier{1, 2, 3}))
		require.NotEmpty(t, ca.Policies)
	})

	t.Run("empty policy OIDs", func(t *testing.T) {
		t.Parallel()

		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		require.Empty(t, ca.PolicyIdentifiers)
		require.Empty(t, ca.Policies)
	})

	t.Run("multiple valid policy OIDs", func(t *testing.T) {
		t.Parallel()

		policies := []asn1.ObjectIdentifier{
			{2, 5, 29, 32, 0},
			{2, 5, 29, 32, 1},
			{2, 5, 29, 32, 2},
		}

		_, certder, err := NewRSAPrikeyAndCert(RSAPrikeyBits3072,
			WithX509CertCommonName("laisky-test"),
			WithX509CertPolicies(policies...),
		)
		require.NoError(t, err)

		ca, err := Der2Cert(certder)
		require.NoError(t, err)

		require.Len(t, ca.PolicyIdentifiers, len(policies))
		for _, policy := range policies {
			require.Contains(t, ca.PolicyIdentifiers, policy)
			oid, err := OidAsn2X509(policy)
			require.NoError(t, err)
			require.Contains(t, ca.Policies, oid)
		}
		require.Len(t, ca.Policies, len(policies))
	})
}

func TestOidFromString(t *testing.T) {
	t.Parallel()

	input := "1.2.3.4"
	oid, err := OidFromString(input)
	require.NoError(t, err)
	require.True(t, oid.EqualASN1OID(asn1.ObjectIdentifier{1, 2, 3, 4}))
}
