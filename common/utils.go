// Package common global shared utils
package common

import "unsafe"

// Str2Bytes unsafe convert str to bytes
func Str2Bytes(s string) []byte {
	sp := (*[2]uintptr)(unsafe.Pointer(&s))
	bp := [3]uintptr{sp[0], sp[1], sp[1]}
	return *(*[]byte)(unsafe.Pointer(&bp))
}

// Bytes2Str unsafe convert bytes to str
func Bytes2Str(b []byte) string {
	bp := (*[3]uintptr)(unsafe.Pointer(&b))
	sp := [2]uintptr{bp[0], bp[1]}
	return *(*string)(unsafe.Pointer(&sp))
}
