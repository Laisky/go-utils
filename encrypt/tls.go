package encrypt

import "crypto/tls"

// SecureCipherSuites get golang built-in cipher suites without known insecure suites
func SecureCipherSuites(filter func(*tls.CipherSuite) bool) []uint16 {
	var cs []uint16
	for _, s := range tls.CipherSuites() {
		if filter == nil || filter(s) {
			cs = append(cs, s.ID)
		}
	}

	return cs
}
