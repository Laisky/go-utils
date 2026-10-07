package utils

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// NewJaegerTracingID generate jaeger tracing id
//
// Args:
//   - traceID: trace id, 64bit number, will encode to hex string
//   - spanID: span id, 64bit number, will encode to hex string
//   - parentSpanID: parent span id, 64bit number, will encode to hex string
//   - flag: 8bit number, one byte bitmap, as one or two hex digits (leading zero may be omitted)
//
// Even if some of the parameters have incorrect formatting,
// it won't result in an error; instead, it will generate a new random value.
func NewJaegerTracingID(traceID, spanID, parentSpanID uint64, flag byte) (traceVal JaegerTracingID, err error) {
	if traceID == 0 {
		if traceID, err = RandomNonZeroUint64(); err != nil {
			return "", errors.Wrapf(err, "generate random trace id")
		}
	}
	if spanID == 0 {
		if spanID, err = RandomNonZeroUint64(); err != nil {
			return "", errors.Wrapf(err, "generate random span id")
		}
	}
	if flag == 0 {
		flag = 0x04 // default to not used
	}

	traceIDVal := strconv.FormatUint(traceID, 16)
	spanIDVal := strconv.FormatUint(spanID, 16)
	parentSpanIDVal := strings.TrimLeft(fmt.Sprintf("%016x", parentSpanID), "0")
	flagVal := strconv.FormatUint(uint64(flag), 16)

	return JaegerTracingID(fmt.Sprintf("%s:%s:%s:%s", traceIDVal, spanIDVal, parentSpanIDVal, flagVal)), nil
}

// PaddingLeft padding string to left
func PaddingLeft(s string, padStr string, pLen int) string {
	if len(s) >= pLen {
		return s
	}

	return strings.Repeat(padStr, pLen-len(s)) + s
}

// JaegerTracingID jaeger tracing id
type JaegerTracingID string

// String implement fmt.Stringer
func (t JaegerTracingID) String() string {
	return string(t)
}

// Parse decodes this API's 64-bit Jaeger IDs and one-byte flags. It rejects input
// exceeding the fixed 53-byte format before splitting or decoding. An empty
// parent ID remains accepted as zero for IDs emitted by older versions.
//
// A zero trace ID or a zero span ID is rejected, because the uber-trace-id
// format defines both as invalid and NewJaegerTracingID never emits them; a
// zero parent span ID (a root span) and a zero flags byte stay valid. On any
// error all returned values are zero.
func (t JaegerTracingID) Parse() (traceID, spanID, parentSpanID uint64, flag byte, err error) {
	if len(t) > 53 {
		return 0, 0, 0, 0, errors.New("invalid trace value: too long")
	}
	var fields [4]string
	remaining := string(t)
	for i := 0; i < 3; i++ {
		var found bool
		fields[i], remaining, found = strings.Cut(remaining, ":")
		if !found {
			return 0, 0, 0, 0, errors.New("invalid trace value: expected four components")
		}
	}
	fields[3] = remaining
	if fields[2] == "" {
		fields[2] = "0"
	}
	var ids [3]uint64
	for i := range ids {
		value, parseErr := parseJaegerComponent(fields[i], i, 16)
		if parseErr != nil {
			return 0, 0, 0, 0, parseErr
		}
		ids[i] = value
	}
	// Positions 0 (trace ID) and 1 (span ID) must be non-zero; position 2 (parent) may be zero.
	for i := range 2 {
		if ids[i] == 0 {
			return 0, 0, 0, 0, errors.Errorf("invalid trace component %d: zero", i)
		}
	}
	flagValue, parseErr := parseJaegerComponent(fields[3], 3, 2)
	if parseErr != nil {
		return 0, 0, 0, 0, parseErr
	}
	// Two hexadecimal digits parsed with bitSize 8 cannot exceed MaxUint8; the
	// explicit bound keeps the byte conversion provably lossless.
	if flagValue > math.MaxUint8 {
		return 0, 0, 0, 0, errors.New("invalid trace component 3: range")
	}
	return ids[0], ids[1], ids[2], byte(flagValue), nil
}

// parseJaegerComponent decodes one Jaeger trace component. It takes the raw
// field, its position (used only in bounded diagnostics that never echo input)
// and the maximum number of hexadecimal digits, and returns the decoded value
// or an error for an empty, overlong, non-hexadecimal or out-of-range field.
func parseJaegerComponent(field string, position, maxDigits int) (uint64, error) {
	if len(field) == 0 || len(field) > maxDigits {
		return 0, errors.Errorf("invalid trace component %d: width", position)
	}
	for i := range len(field) {
		if !isASCIIHexDigit(field[i]) {
			return 0, errors.Errorf("invalid trace component %d: hexadecimal syntax", position)
		}
	}
	value, err := strconv.ParseUint(field, 16, maxDigits*4)
	if err != nil {
		return 0, errors.Errorf("invalid trace component %d: range", position)
	}
	return value, nil
}

// isASCIIHexDigit reports whether c is an ASCII hexadecimal digit in either
// letter case. It takes one byte and returns true only for 0-9, a-f and A-F.
func isASCIIHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// RandomNonZeroUint64 generate random uint64 number
func RandomNonZeroUint64() (uint64, error) {
	var num uint64
	for {
		if err := binary.Read(rand.Reader, binary.BigEndian, &num); err != nil {
			return 0, errors.Wrap(err, "generate random number")
		}

		if num != 0 {
			return num, nil
		}
	}
}

// NewSpan generate new span
//
// The new span keeps the trace ID and flags of t, records the span ID of t as
// its parent and gets a fresh random span ID. It returns an error when t does
// not parse, including when its trace or span ID is zero, instead of starting
// an unrelated trace.
func (t JaegerTracingID) NewSpan() (JaegerTracingID, error) {
	traceID, spanID, _, flag, err := t.Parse()
	if err != nil {
		return "", errors.Wrapf(err, "parse traceID")
	}

	newSpanID, err := RandomNonZeroUint64()
	if err != nil {
		return "", errors.Wrapf(err, "generate new spanID")
	}

	return NewJaegerTracingID(traceID, newSpanID, spanID, flag)
}
