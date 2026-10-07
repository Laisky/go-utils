package crypto

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"net/url"
	"testing"

	"github.com/emmansun/gmsm/smx509"
	"github.com/stretchr/testify/require"
)

// TestTongsuoCloneX509CsrIsStructural clones a CSR whose subject and SAN
// values contain display separators and requires the clone to reproduce the
// original subject and SANs exactly, attribute by attribute and type by type,
// under the new key. ParseCsr2Opts used to split `tongsuo req -text` output,
// which broke on Tongsuo 8.5 formatting and let values inject attributes;
// same root cause as issue #61.
func TestTongsuoCloneX509CsrIsStructural(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)
	ctx := t.Context()

	oldKey, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	newKey, err := ins.NewPrikey(ctx)
	require.NoError(t, err)
	uri, err := url.Parse("https://svc.example/path")
	require.NoError(t, err)

	origDer, err := ins.NewX509CSR(ctx, oldKey,
		WithX509CSRCommonName("leaf, O = injected-org"),
		WithX509CSRCountry("CN"),
		WithX509CSRProvince("Shanghai"),
		WithX509CSRLocality("Shanghai"),
		WithX509CSROrganization("BBT", "Second Org"),
		WithX509CSROrganizationUnit("OU=ops"),
		WithX509CSRStreetAddrs("1 Main St"),
		WithX509CSRPostalCode("200000"),
		WithX509CSRDNSNames("www.example.com", "1.2.3.4", "a.example, DNS:injected.example"),
		WithX509CSREmailAddrs("test@laisky.com"),
		WithX509CSRIPAddrs(net.ParseIP("192.0.2.7")),
		WithX509CSRURIs(uri),
	)
	require.NoError(t, err)
	orig, err := smx509.ParseCertificateRequest(origDer)
	require.NoError(t, err)

	clonedDer, err := ins.CloneX509Csr(ctx, newKey, origDer)
	require.NoError(t, err)
	cloned, err := smx509.ParseCertificateRequest(clonedDer)
	require.NoError(t, err)
	require.NoError(t, cloned.CheckSignature())

	require.Equal(t, orig.Subject.Names, cloned.Subject.Names)
	require.Equal(t, orig.DNSNames, cloned.DNSNames)
	require.Equal(t, orig.EmailAddresses, cloned.EmailAddresses)
	require.Len(t, cloned.IPAddresses, 1)
	require.True(t, orig.IPAddresses[0].Equal(cloned.IPAddresses[0]))
	require.Equal(t, urisToStrings(orig.URIs), urisToStrings(cloned.URIs))

	newPubDer, err := ins.Prikey2Pubkey(ctx, newKey)
	require.NoError(t, err)
	newPub, err := Pem2Der(newPubDer)
	require.NoError(t, err)
	require.Equal(t, newPub, cloned.RawSubjectPublicKeyInfo, "clone must carry the new key")
}

// TestTongsuoParseCsr2OptsRejectsUnsupportedAttributes verifies subject
// attributes that the CSR options cannot reproduce fail closed instead of
// being dropped silently from a clone.
func TestTongsuoParseCsr2OptsRejectsUnsupportedAttributes(t *testing.T) {
	t.Parallel()
	ins := newSecurityTestTongsuo(t)

	key, err := NewECDSAPrikey(ECDSACurveP256)
	require.NoError(t, err)
	csrDer, err := NewX509CSR(key, WithX509CSRSubject(pkix.Name{
		CommonName: "with-email-attribute",
		ExtraNames: []pkix.AttributeTypeAndValue{{
			Type:  asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1},
			Value: "admin@example.com",
		}},
	}))
	require.NoError(t, err)

	opts, err := ins.ParseCsr2Opts(t.Context(), csrDer)
	require.Error(t, err)
	require.Nil(t, opts)

	opts, err = ins.ParseCsr2Opts(t.Context(), []byte("not a csr"))
	require.Error(t, err)
	require.Nil(t, opts)
}
