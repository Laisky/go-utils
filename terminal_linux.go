//go:build linux

package utils

import (
	"fmt"
	"syscall"

	"github.com/Laisky/errors/v2"
	"golang.org/x/term"
)

// InputPassword reads password from stdin input
// and returns it as a string.
func InputPassword(hint string, validator func(string) error) (passwd string, err error) {
	fmt.Printf("%s: \n", hint)

	for {
		bytepw, err := term.ReadPassword(syscall.Stdin)
		if err != nil {
			return "", errors.Wrap(err, "read input password")
		}

		if validator == nil {
			return string(bytepw), nil
		}

		if err := validator(string(bytepw)); err != nil {
			fmt.Printf("invalid password: %s\n", err.Error())
			fmt.Printf("try again: \n")
			continue
		}

		return string(bytepw), nil
	}
}
