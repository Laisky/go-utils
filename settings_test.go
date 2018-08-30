package utils_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/Laisky/go-utils"
)

func TestSettings(t *testing.T) {
	var (
		err error
		st  = []byte(`---
key1: val1
key2: val2
key3: val3`)
	)

	dirName, err := ioutil.TempDir("", "go-utils-test")
	if err != nil {
		t.Fatalf("try to create tmp dir got error: %+v", err)
	}
	fp, err := os.Create(filepath.Join(dirName, "settings.yml"))
	if err != nil {
		t.Fatalf("try to create tmp file got error: %+v", err)
	}
	t.Logf("create file: %v", fp.Name())
	// defer os.RemoveAll(dirName)

	fp.Write(st)
	fp.Close()

	t.Logf("load settings from: %v", dirName)
	if err = utils.Settings.Setup(dirName); err != nil {
		t.Fatalf("setup settings got error: %+v", err)
	}

	t.Logf(">> key1: %+v", viper.Get("key1"))
	fp, err = os.Open(fp.Name())
	defer fp.Close()
	if b, err := ioutil.ReadAll(fp); err != nil {
		t.Fatalf("try to read tmp file got error: %+v", err)
	} else {
		t.Logf("file content: %v", string(b))
	}

	cases := map[string]string{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
	}
	var got string
	for k, expect := range cases {
		got = utils.Settings.GetString(k)
		if got != expect {
			t.Errorf("load %v, expect %v, got %v", k, expect, got)
		}
	}
}
