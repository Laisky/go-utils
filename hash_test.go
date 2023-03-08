package utils

import (
	"testing"

	"github.com/Laisky/go-utils/v4/log"
	"github.com/Laisky/zap"
)

const (
	testhashraw = "dfij3ifj2jjl2jelkjdkwef"
)

func TestHashSHA128String(t *testing.T) {
	val := testhashraw
	got := HashSHA128String(val)
	if got != "57dce855bbee0bef97b63527d473c807a424511d" {
		t.Fatalf("got: %v", got)
	}
}
func ExampleHashSHA128String() {
	val := testhashraw
	got := HashSHA128String(val)
	log.Shared.Info("hash", zap.String("got", got))
}

func TestHashSHA256String(t *testing.T) {
	val := testhashraw
	got := HashSHA256String(val)
	if got != "fef14c65b3d411fee6b2dbcb791a9536cbf637b153bb1de0aae1b41e3834aebf" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashSHA256String() {
	val := testhashraw
	got := HashSHA256String(val)
	log.Shared.Info("hash", zap.String("got", got))
}

func TestHashXxhashString(t *testing.T) {
	val := testhashraw
	got := HashXxhashString(val)
	if got != "6466696a3369666a326a6a6c326a656c6b6a646b776566ef46db3751d8e999" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashXxhashString() {
	val := testhashraw
	got := HashXxhashString(val)
	log.Shared.Info("hash", zap.String("got", got))
}
