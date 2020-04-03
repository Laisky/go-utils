package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func ExampleSettings() {
	// read settings from yml file
	pflag.String("config", "/etc/go-ramjet/settings", "config file directory path")
	pflag.Parse()

	// bind pflags to settings
	if err := Settings.BindPFlags(pflag.CommandLine); err != nil {
		Logger.Panic("parse command")
	}

	// use
	Settings.Get("xxx")
	Settings.GetString("xxx")
	Settings.GetStringSlice("xxx")
	Settings.GetBool("xxx")

	Settings.Set("name", "val")
}

func ExampleSettings_cobra() {
	/*

		import {
			"github.com/spf13/cobra"
		}

		// with cobra command
		rootCmd := &cobra.Command{}
		childCmd := &cobra.Command{
			PreRun: func(cmd *cobra.Command, args []string) {
				if err := Settings.BindPFlags(cmd.Flags()); err != nil {
					Logger.Panic("parse args")
				}
			},
		}

		rootCmd.AddCommand(childCmd)
		childCmd.Flags().BoolP("verbose", "v", false, "verbose")

		fmt.Println(Settings.GetBool("verbose"))
		// Output: false

	*/
}

func TestSettings(t *testing.T) {
	var (
		err error
		st  = []byte(`---
key1: val1
key2: val2
key3: val3
key4:
  k4.1: 12
  k4.2: "qq"
  k4.3: "123 : 123"
`)
	)

	dirName, err := ioutil.TempDir("", "go-utils-test-settings")
	if err != nil {
		t.Fatalf("try to create tmp dir got error: %+v", err)
	}
	fp, err := os.Create(filepath.Join(dirName, "settings.yml"))
	if err != nil {
		t.Fatalf("try to create tmp file got error: %+v", err)
	}
	t.Logf("create file: %v", fp.Name())
	defer os.RemoveAll(dirName)

	if _, err = fp.Write(st); err != nil {
		t.Fatalf("%+v", err)
	}
	if err = fp.Close(); err != nil {
		t.Fatalf("%+v", err)
	}

	t.Logf("load settings from: %v", dirName)
	if err = Settings.Setup(dirName); err != nil {
		t.Fatalf("setup settings got error: %+v", err)
	}

	t.Logf(">> key1: %+v", viper.Get("key1"))
	if fp, err = os.Open(fp.Name()); err != nil {
		t.Fatalf("open: %+v", err)
	}
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
		got = Settings.GetString(k)
		if got != expect {
			t.Errorf("load %v, expect %v, got %v", k, expect, got)
		}
	}

	mr := Settings.GetStringMapString("key4")
	if mr["k4.1"] != "12" ||
		mr["k4.2"] != "qq" ||
		mr["k4.3"] != "123 : 123" {
		t.Fatalf("string map string got %+v", mr)
	}
}

// depended on remote config-s  erver
func TestSetupFromConfigServerWithRawYaml(t *testing.T) {
	fakedata := map[string]interface{}{
		"name":     "app",
		"profiles": []string{"profile"},
		"label":    "label",
		"version":  "12345",
		"propertySources": []map[string]interface{}{
			{
				"name": "config name",
				"source": map[string]string{
					"profile": "profile",
					"raw": `
a:
  b: 123
  c: abc
  d:
    - 1
    - 2
  e: true`,
				},
			},
		},
	}

	// jb, err := json.Marshal(fakedata)
	// if err != nil {
	// 	Logger.Panic("try to marshal fake data got error", zap.Error(err))
	// }
	port := 24953
	addr := fmt.Sprintf("http://localhost:%v", port)
	go RunMockConfigSrv(port, fakedata)
	time.Sleep(100 * time.Millisecond)
	if err := Settings.SetupFromConfigServerWithRawYaml(addr, "app", "profile", "label", "raw"); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	for k, vi := range map[string]interface{}{
		"a.b": 123,
		"a.c": "abc",
		"a.d": []string{"1", "2"},
		"a.e": true,
	} {
		switch val := vi.(type) {
		case string:
			if Settings.GetString(k) != val {
				t.Fatalf("`%v` should be `%v`, but got %+v", k, val, Settings.Get(k))
			}
		case int:
			if Settings.GetInt(k) != val {
				t.Fatalf("`%v` should be `%v`, but got %+v", k, val, Settings.Get(k))
			}
		case []string:
			vs := Settings.GetStringSlice(k)
			if len(vs) != 2 ||
				vs[0] != val[0] ||
				vs[1] != val[1] {
				t.Fatalf("`%v` should be `%v`, but got %+v", k, val, Settings.Get(k))
			}
		case bool:
			if Settings.GetBool(k) != val {
				t.Fatalf("`%v` should be `%v`, but got %+v", k, val, Settings.Get(k))
			}
		default:
			t.Fatal("unknown type")
		}
	}
}

func BenchmarkSettings(b *testing.B) {
	b.Run("set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Settings.Set(RandomStringWithLength(20), RandomStringWithLength(20))
		}
	})
	b.Run("get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Settings.Get(RandomStringWithLength(20))
		}
	})
}
