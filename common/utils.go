// Package common global shared utils
package common

import "unsafe"

// Str2Bytes converts s to a byte slice that shares s's memory without copying.
// The returned slice has len and cap equal to len(s) and must never be
// modified, because Go strings are immutable. An empty s yields an empty slice.
//
// It uses unsafe.Slice and unsafe.StringData, the conversions sanctioned by the
// unsafe package rules, instead of reinterpreting header words through uintptr
// arrays, which the garbage collector does not track as pointers.
//
//nolint:gosec // G103: deliberate zero-copy conversion; callers must not mutate the result.
func Str2Bytes(s string) []byte {
	if len(s) == 0 {
		return []byte{}
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// Bytes2Str converts b to a string that shares b's memory without copying. The
// caller must not modify b afterwards, because the string would change too.
// An empty b yields "".
//
//nolint:gosec // G103: deliberate zero-copy conversion; callers must not mutate the input.
func Bytes2Str(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}
