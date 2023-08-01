package utils

import (
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/Laisky/errors/v2"
	"golang.org/x/term"
)

// InputPassword reads password from stdin input
// and returns it as a string.
func InputPassword(hint string, validator func(string) error) (passwd string, err error) {
	fmt.Printf("%s: ", hint)

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
			fmt.Printf("try again: ")
			continue
		}

		return string(bytepw), nil
	}
}

// InputYes require user input `y` or `Y` to continue
func InputYes(hint string) (ok bool, err error) {
	fmt.Printf("%s, input y/Y to continue: ", hint)

	var confirm string
	_, err = fmt.Scanln(&confirm)
	if err != nil {
		if err.Error() == "unexpected newline" || errors.Is(err, io.EOF) {
			// user input nothing, use default value
			confirm = "y"
		} else {
			return ok, errors.Wrap(err, "read input")
		}
	}

	if strings.ToLower(confirm) != "y" {
		return false, nil
	}

	return true, nil
}

// Input reads input from stdin
func Input(hint string) (input string, err error) {
	fmt.Printf("%s: ", hint)

	_, err = fmt.Scanln(&input)
	if err != nil {
		return "", errors.Wrap(err, "read input")
	}

	return input, nil
}
