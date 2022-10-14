package utils

import (
	crand "crypto/rand"
	"math/big"
	"math/rand"
	"os"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano() + int64(os.Getpid()))
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// NewRand new individual random to aviod global mutex
func NewRand() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// RandomBytesWithLength generate random bytes
func RandomBytesWithLength(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

// SecRandomBytesWithLength generate crypto random bytes
func SecRandomBytesWithLength(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := crand.Read(b)
	return b, err
}

// RandomStringWithLength generate random string with specific length
func RandomStringWithLength(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// SecRandomStringWithLength generate random string with specific length
func SecRandomStringWithLength(n int) (string, error) {
	b := make([]rune, n)
	for i := range b {
		idx, err := SecRandInt(len(letterRunes))
		if err != nil {
			return "", err
		}

		b[i] = letterRunes[idx]
	}

	return string(b), nil
}

// SecRandInt generate security int
func SecRandInt(n int) (int, error) {
	bn, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}

	return int(bn.Int64()), nil
}
