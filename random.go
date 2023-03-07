package utils

import (
	crand "crypto/rand"
	"math/big"
	"math/rand"
	"sync"
	"time"
)

var (
	randor   = NewRand()
	randorMu sync.Mutex

	letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

// NewRand new individual random to aviod global mutex
func NewRand() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// RandomBytesWithLength generate random bytes
func RandomBytesWithLength(n int) ([]byte, error) {
	b := make([]byte, n)

	randorMu.Lock()
	_, err := randor.Read(b)
	randorMu.Unlock()

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

// RandomChoice selects a random subset of elements from an input array of any type.
//
// It takes in two parameters: the array and the number of elements to select from the array.
// The function uses a random number generator to select elements from the array and
// returns a new array containing the selected elements.
func RandomChoice[T any](arr []T, n int) (got []T) {
	if n >= len(arr) {
		return arr
	} else if n == 0 {
		return got
	}

	randor := NewRand()
	thres := float64(n) / float64(len(arr))
	for i := range arr {
		if (len(arr) - i) <= (n - len(got)) {
			return append(got, arr[i:]...)
		}

		if randor.Float64() < thres {
			got = append(got, arr[i])
		}

		if len(got) == n {
			return got
		}
	}

	return got
}
