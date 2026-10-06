package log

import (
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// validateRotationFilename accepts a single portable filename, never a path.
// Windows device names and alternate data streams are rejected on every host.
// This is lexical validation only; it does not enforce a destination link policy.
func validateRotationFilename(name string) error {
	if name == "" || name == "." || name == ".." || len(name) > 255 || !utf8.ValidString(name) ||
		strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") || strings.ContainsAny(name, `/\:<>"|?*`) {
		return errors.New("rotation pattern must produce one portable filename of 1 to 255 bytes")
	}
	for _, char := range name {
		if char < 32 || char == 127 {
			return errors.New("rotation filename must not contain control characters")
		}
	}
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "CLOCK$":
		return errors.New("rotation filename must not name a Windows device")
	}
	if strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT") {
		suffix := []rune(stem[3:])
		if len(suffix) == 1 && (suffix[0] >= '1' && suffix[0] <= '9' || strings.ContainsRune("¹²³", suffix[0])) {
			return errors.New("rotation filename must not name a Windows device")
		}
	}
	return nil
}
