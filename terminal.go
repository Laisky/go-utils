package utils

import (
	"fmt"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
)

// InputYes require user input `y` or `Y` to continue
func InputYes(hint string) (ok bool, err error) {
	fmt.Printf("%s, input y/Y to continue: \n", hint)

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
	fmt.Printf("%s: \n", hint)

	_, err = fmt.Scanln(&input)
	if err != nil {
		return "", errors.Wrap(err, "read input")
	}

	return input, nil
}
