package log

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

type patternElementKind int

const (
	patternLiteral patternElementKind = iota
	patternLogger
	patternYear
	patternMonth
	patternDay
	patternHour
	patternMinute
	patternSecond
)

type patternElement struct {
	kind    patternElementKind
	literal string
}

type rotationPattern struct {
	raw      string
	elements []patternElement
	hasYear  bool
	hasMonth bool
	hasDay   bool
}

// Path resolves a validated filename beneath baseDir without an absolute-path bypass.
func (rp *rotationPattern) Path(logger string, start time.Time, baseDir string) (string, error) {
	name := rp.format(logger, start)
	if err := validateRotationFilename(name); err != nil {
		return "", errors.Wrap(err, "validate formatted rotation filename")
	}
	if baseDir == "" {
		baseDir = "."
	}
	result := filepath.Join(baseDir, name)
	relative, err := filepath.Rel(baseDir, result)
	if err != nil {
		return "", errors.Wrap(err, "resolve rotation filename")
	}
	if !filepath.IsLocal(relative) {
		return "", errors.New("rotation filename escaped its base directory")
	}
	return result, nil
}

// format renders the pattern for logger and start, replacing {logger} with logger and each
// date or time token with the matching zero-padded field of start. It returns the
// resulting filename without validating it.
func (rp *rotationPattern) format(logger string, start time.Time) string {
	var b strings.Builder
	for _, el := range rp.elements {
		switch el.kind {
		case patternLiteral:
			b.WriteString(el.literal)
		case patternLogger:
			b.WriteString(logger)
		case patternYear:
			_, _ = fmt.Fprintf(&b, "%04d", start.Year())
		case patternMonth:
			_, _ = fmt.Fprintf(&b, "%02d", int(start.Month()))
		case patternDay:
			_, _ = fmt.Fprintf(&b, "%02d", start.Day())
		case patternHour:
			_, _ = fmt.Fprintf(&b, "%02d", start.Hour())
		case patternMinute:
			_, _ = fmt.Fprintf(&b, "%02d", start.Minute())
		case patternSecond:
			_, _ = fmt.Fprintf(&b, "%02d", start.Second())
		}
	}

	return b.String()
}

// Parse matches name against the pattern for the given logger and extracts the encoded
// timestamp; hour, minute and second default to zero when the pattern omits them. It
// returns the UTC time and true when name matches exactly and every field is in range, or
// the zero time and false otherwise.
func (rp *rotationPattern) Parse(name string, logger string) (time.Time, bool) {
	pos := 0
	parts := timeParts{year: -1, month: -1, day: -1, hour: 0, minute: 0, second: 0}

	for _, el := range rp.elements {
		switch el.kind {
		case patternLiteral:
			if !strings.HasPrefix(name[pos:], el.literal) {
				return time.Time{}, false
			}
			pos += len(el.literal)
		case patternLogger:
			if !strings.HasPrefix(name[pos:], logger) {
				return time.Time{}, false
			}
			pos += len(logger)
		case patternYear:
			value, ok := parseFourDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.year = value
			pos += 4
		case patternMonth:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.month = value
			pos += 2
		case patternDay:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.day = value
			pos += 2
		case patternHour:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.hour = value
			pos += 2
		case patternMinute:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.minute = value
			pos += 2
		case patternSecond:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.second = value
			pos += 2
		}
	}

	if pos != len(name) {
		return time.Time{}, false
	}

	if parts.year < 0 || parts.month < 1 || parts.day < 1 {
		return time.Time{}, false
	}

	if parts.month > 12 || parts.day > 31 || parts.hour > 23 || parts.minute > 59 || parts.second > 59 {
		return time.Time{}, false
	}

	result := time.Date(
		parts.year,
		time.Month(parts.month),
		parts.day,
		parts.hour,
		parts.minute,
		parts.second,
		0,
		time.UTC,
	)
	return result, true
}

type timeParts struct {
	year   int
	month  int
	day    int
	hour   int
	minute int
	second int
}

// parseFourDigits parses the four bytes of input starting at pos as a decimal number. It
// returns the value and true, or 0 and false when fewer than four bytes remain or any of
// them is not an ASCII digit.
func parseFourDigits(input string, pos int) (int, bool) {
	if pos+4 > len(input) {
		return 0, false
	}
	return parseDigits(input[pos : pos+4])
}

// parseTwoDigits parses the two bytes of input starting at pos as a decimal number. It
// returns the value and true, or 0 and false when fewer than two bytes remain or either of
// them is not an ASCII digit.
func parseTwoDigits(input string, pos int) (int, bool) {
	if pos+2 > len(input) {
		return 0, false
	}
	return parseDigits(input[pos : pos+2])
}

// parseDigits parses segment as an unsigned decimal number. It returns the value and true,
// or 0 and false when segment contains a rune that is not an ASCII digit.
func parseDigits(segment string) (int, bool) {
	value := 0
	for _, r := range segment {
		if r < '0' || r > '9' {
			return 0, false
		}
		value = value*10 + int(r-'0')
	}
	return value, true
}

// compileRotationPattern tokenizes the trimmed pattern into {logger}, date and time tokens
// and literal runs. Literal runs must not contain path separators, the YYYY, MM and DD
// tokens are mandatory, and a sample rendering must be a valid portable filename. It
// returns the compiled pattern, or an error when pattern is blank or breaks these rules.
func compileRotationPattern(pattern string) (*rotationPattern, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.Errorf("rotation filename pattern must not be empty")
	}

	rp := &rotationPattern{raw: pattern}

	for i := 0; i < len(pattern); {
		if strings.HasPrefix(pattern[i:], "{logger}") {
			rp.elements = append(rp.elements, patternElement{kind: patternLogger})
			i += len("{logger}")
			continue
		}

		if token, kind := matchPatternToken(pattern[i:]); token != "" {
			rp.elements = append(rp.elements, patternElement{kind: kind})
			i += len(token)
			switch kind {
			case patternLiteral:
			case patternYear:
				rp.hasYear = true
			case patternMonth:
				rp.hasMonth = true
			case patternDay:
				rp.hasDay = true
			case patternLogger:
			case patternHour:
			case patternMinute:
			case patternSecond:
			}
			continue
		}

		j := i + 1
		for j < len(pattern) {
			if strings.HasPrefix(pattern[j:], "{logger}") {
				break
			}
			if token, _ := matchPatternToken(pattern[j:]); token != "" {
				break
			}
			j++
		}
		literal := pattern[i:j]
		if strings.ContainsAny(literal, `/\`) {
			return nil, errors.Errorf("rotation filename pattern must not contain path separators")
		}
		rp.elements = append(rp.elements, patternElement{kind: patternLiteral, literal: literal})
		i = j
	}

	if !rp.hasYear || !rp.hasMonth || !rp.hasDay {
		return nil, errors.Errorf("rotation filename pattern must include YYYY, MM, and DD tokens")
	}

	if err := validateRotationFilename(rp.format("logger", time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC))); err != nil {
		return nil, errors.Wrap(err, "validate rotation filename pattern")
	}
	return rp, nil
}

// matchPatternToken reports which date or time token, if any, starts input; recognized
// tokens are YYYY, MM, DD, hh or HH, mm and ss. It returns the matched token text and its
// element kind, or an empty string and zero when input does not start with a token.
func matchPatternToken(input string) (string, patternElementKind) {
	tokens := []struct {
		text string
		kind patternElementKind
	}{
		{"YYYY", patternYear},
		{"MM", patternMonth},
		{"DD", patternDay},
		{"hh", patternHour},
		{"HH", patternHour},
		{"mm", patternMinute},
		{"ss", patternSecond},
	}

	for _, token := range tokens {
		if strings.HasPrefix(input, token.text) {
			return token.text, token.kind
		}
	}

	return "", 0
}
