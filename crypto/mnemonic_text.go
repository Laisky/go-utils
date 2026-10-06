package crypto

import (
	"unicode"

	"github.com/Laisky/errors/v2"
)

const (
	// Extended encoding is capped at 65,535 payload bytes. One MiB permits
	// its entire canonical English-word representation plus ample whitespace.
	maxMnemonicTextBytes = 1 << 20
	maxMnemonicWordBytes = 8 // Longest word in the BIP39 English list.
)

// splitBoundedMnemonic tokenizes extended-format English mnemonics within fixed
// text, token-count, and token-length budgets. Limits precede slice growth and
// word-map lookups. Errors identify positions without returning mnemonic words.
func splitBoundedMnemonic(text string) ([]string, error) {
	if len(text) > maxMnemonicTextBytes {
		return nil, errors.New("extended mnemonic text exceeds byte limit")
	}
	words := make([]string, 0, 32)
	start := -1
	for offset, r := range text {
		if unicode.IsSpace(r) {
			if start >= 0 {
				var err error
				words, err = appendBoundedMnemonicWord(words, text[start:offset])
				if err != nil {
					return nil, errors.Wrap(err, "split extended mnemonic")
				}
				start = -1
			}
			continue
		}
		if start < 0 {
			start = offset
		}
		if offset-start >= maxMnemonicWordBytes {
			return nil, errors.Errorf("invalid mnemonic word length at index %d", len(words))
		}
	}
	if start >= 0 {
		var err error
		words, err = appendBoundedMnemonicWord(words, text[start:])
		if err != nil {
			return nil, errors.Wrap(err, "split extended mnemonic")
		}
	}
	if len(words) == 0 {
		return nil, errors.New("mnemonic must not be empty")
	}
	return words, nil
}

// appendBoundedMnemonicWord validates one token before extending the token slice.
func appendBoundedMnemonicWord(words []string, word string) ([]string, error) {
	if len(words) >= maxEncodedMnemonicWords {
		return nil, errors.New("mnemonic too long: exceeds word limit")
	}
	if len(word) == 0 || len(word) > maxMnemonicWordBytes {
		return nil, errors.Errorf("invalid mnemonic word length at index %d", len(words))
	}
	return append(words, word), nil
}
