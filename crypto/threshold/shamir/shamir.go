// Package shamir is Shamir’s Secret Sharing is a method for
// dividing a secret into multiple parts and distributing those parts among
// different participants. This ensures that only authorized parties can access the secret.
//
// The method involves randomly generating n parts, each of which is
// completely independent of the other parts and contains
// no information about the original secret. An integer threshold k is
// specified such that the original secret can only be
// reconstructed when k or more parts are combined together.
//
// Each participant holds one part of the secret and can only access
// the original secret when they combine their part with at
// least k-1 other parts. This method provides high security, as even if
// some of the participants are compromised, they will
// only have access to a partial key, and cannot reconstruct the original secret.
package shamir

import (
	"github.com/Laisky/errors/v2"
	"github.com/corvus-ch/shamir"
)

// Split takes an arbitrarily long secret and generates a `total`
// number of shares, `threshold` of which are required to reconstruct
// the secret. The total and threshold must be at least 2, and less
// than 256. The returned shares are each one byte longer than the secret
// as they attach a tag used to reconstruct the secret.
//
// the key and values of members are both important to combine.
func Split(secret []byte, total, threshold int) (members map[byte][]byte, err error) {
	switch {
	case threshold < 2 || threshold >= 256:
		return nil, errors.Errorf("threshold shoule be in [2, 256) got %d", threshold)
	case total < 2 || total >= 256:
		return nil, errors.Errorf("total shoule be in [2, 256) got %d", total)
	case total <= threshold:
		return nil, errors.Errorf("total should greater than threshold")
	}

	if members, err = shamir.Split(secret, total, threshold); err != nil {
		return nil, errors.Wrap(err, "split")
	}

	return members, nil
}

// Combine is used to reverse a Split and reconstruct a secret
// once a `threshold` number of parts are available.
//
// the key and value are must as same as splited result.
func Combine(parts map[byte][]byte) ([]byte, error) {
	if len(parts) < 2 {
		return nil, errors.Errorf("length of parts should >= 2")
	}

	return shamir.Combine(parts)
}
