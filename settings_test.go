package utils_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/go-utils"
	"github.com/pkg/errors"
)

func TestSettings(t *testing.T) {
	cases := map[string]string{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
	}
	var got string
	for k, expect := range cases {
		got = utils.Settings.GetString(k)
		if got != expect {
			t.Errorf("expect %v, got %v", expect, got)
		}
	}
}

func init() {
	var (
		err          error
		settingsPath = "/tmp/go-utils-testing"
		st           = []byte(`---
key1: val1
key2: val2
key3: val3`)
	)
	err = os.MkdirAll(settingsPath, 0755)
	if err != nil {
		panic(errors.Wrap(err, "try to create tesing directory error"))
	}

	err = ioutil.WriteFile(filepath.Join(settingsPath, "settings.yml"), st, 0644)
	if err != nil {
		panic(err.Error())
	}

	utils.Settings.Setup(settingsPath)
}
