package utils

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
)

func ExampleSettings_BindPFlags() {
	// read settings from yml file
	pflag.String("config", "/etc/go-ramjet/settings", "config file directory path")
	pflag.Parse()

	// bind pflags to settings
	if err := Settings.BindPFlags(pflag.CommandLine); err != nil {
		panic(err)
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
				 Settings.BindPFlags(cmd.Flags()); err != nil {
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
k5: 14
`)
	)

	dirName, err := ioutil.TempDir("", "go-utils-test-settings")
	require.NoError(t, err)
	fp, err := os.Create(filepath.Join(dirName, "settings.yml"))
	require.NoError(t, err)
	t.Logf("create file: %v", fp.Name())
	defer os.RemoveAll(dirName)

	_, err = fp.Write(st)
	require.NoError(t, err)
	require.NoError(t, fp.Close())

	t.Logf("load settings from: %v", dirName)
	err = Settings.LoadFromDir(dirName)
	require.NoError(t, err)

	t.Logf(">> key1: %+v", viper.Get("key1"))
	fp, err = os.Open(fp.Name())
	require.NoError(t, err)
	defer fp.Close()

	b, err := ioutil.ReadAll(fp)
	require.NoError(t, err)
	t.Logf("file content: %v", string(b))

	cases := map[string]string{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
	}
	for k, expect := range cases {
		got := Settings.GetString(k)
		require.Equal(t, expect, got)

		goti := Settings.Get(k)
		require.Equal(t, expect, goti.(string))
	}

	require.Equal(t, int64(12), Settings.GetInt64("key4.k4.1"))
	require.Equal(t, time.Duration(12), Settings.GetDuration("key4.k4.1"))
	require.Equal(t, int64(14), Settings.GetInt64("k5"))
	require.Equal(t, time.Duration(14), Settings.GetDuration("k5"))

	// case: GetStringMapString
	{
		mr := Settings.GetStringMapString("key4")
		require.Equal(t, "12", mr["k4.1"])
		require.Equal(t, "qq", mr["k4.2"])
		require.Equal(t, "123 : 123", mr["k4.3"])
	}

	// case: GetStringMap
	{
		mr := Settings.GetStringMap("key4")
		require.Equal(t, 12, mr["k4.1"])
		require.Equal(t, "qq", mr["k4.2"])
		require.Equal(t, "123 : 123", mr["k4.3"])
	}

	// case: set
	{
		Settings.Set("kkz", 123)
		require.Equal(t, int64(123), Settings.GetInt64("kkz"))
		require.Equal(t, time.Duration(123), Settings.GetDuration("kkz"))

		ok := Settings.IsSet("kkz")
		require.True(t, ok)
		ok = Settings.IsSet(RandomStringWithLength(100))
		require.False(t, ok)
	}
}

func TestSettingsToml(t *testing.T) {
	var (
		err error
		st  = []byte(Dedent(`
			root = "root"

			[foo]
				a = 1
				b = "b"
				c = true
		`))
	)

	dirName, err := ioutil.TempDir("", "go-utils-test-settings")
	require.NoError(t, err)
	defer os.RemoveAll(dirName)

	fp, err := os.Create(filepath.Join(dirName, "settings.toml"))
	require.NoError(t, err)
	t.Logf("create file: %v", fp.Name())

	_, err = fp.Write(st)
	require.NoError(t, err)
	require.NoError(t, fp.Close())

	t.Logf("load settings from: %v", fp.Name())
	err = Settings.LoadFromFile(fp.Name(),
		WithSettingsInclude(true),
		WithSettingsEnableInclude(),
	)
	require.NoError(t, err)

	t.Logf(">> key1: %+v", viper.Get("root"))
	fp, err = os.Open(fp.Name())
	require.NoError(t, err)
	defer fp.Close()

	b, err := ioutil.ReadAll(fp)
	require.NoError(t, err)
	t.Logf("file content: %v", string(b))

	require.Equal(t, "root", Settings.GetString("root"))
	require.Equal(t, 1, Settings.GetInt("foo.a"))
	require.Equal(t, "b", Settings.GetString("foo.b"))
	require.Equal(t, true, Settings.GetBool("foo.c"))
}

// depended on remote config-s  erver
func TestSetupFromConfigServerWithRawYaml(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	// require.NoError(t, err)
	port := 24953
	addr := fmt.Sprintf("http://localhost:%v", port)
	go runMockHTTPServer(ctx, port, "/app/profile/label", fakedata)
	time.Sleep(100 * time.Millisecond)
	err := Settings.LoadFromConfigServerWithRawYaml(addr, "app", "profile", "label", "raw")
	require.NoError(t, err)

	for k, vi := range map[string]interface{}{
		"a.b": 123,
		"a.c": "abc",
		"a.d": []string{"1", "2"},
		"a.e": true,
	} {
		switch val := vi.(type) {
		case string:
			require.Equal(t, val, Settings.GetString(k))
		case int:
			require.Equal(t, val, Settings.GetInt(k))
		case []string:
			vs := Settings.GetStringSlice(k)
			if len(vs) != 2 ||
				vs[0] != val[0] ||
				vs[1] != val[1] {
				t.Fatalf("`%v` should be `%v`, but got %+v", k, val, Settings.Get(k))
			}
		case bool:
			require.Equal(t, val, Settings.GetBool(k))
		default:
			t.Fatal("unknown type")
		}
	}

	type cfgStruct struct {
		A struct {
			B uint   `mapstructure:"b"`
			C string `mapstructure:"c"`
			D []int  `mapstructure:"d"`
			E bool   `mapstructure:"e"`
		}
	}
	cfg := &cfgStruct{}
	err = Settings.Unmarshal(cfg)
	require.NoError(t, err)
	require.Equal(t, uint(123), cfg.A.B)
	require.Equal(t, "abc", cfg.A.C)
	require.True(t, cfg.A.E)

	// case: unmarshal key
	{
		type cfgStruct struct {
			B uint   `mapstructure:"b"`
			C string `mapstructure:"c"`
			D []int  `mapstructure:"d"`
			E bool   `mapstructure:"e"`
		}
		cfg := &cfgStruct{}
		err := Settings.UnmarshalKey("a", cfg)
		require.NoError(t, err)
		require.Equal(t, uint(123), cfg.B)
		require.Equal(t, "abc", cfg.C)
		require.True(t, cfg.E)

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

func TestAESEncryptFilesInDir(t *testing.T) {
	dirName, err := ioutil.TempDir("", "go-utils-test-settings")
	require.NoError(t, err)
	defer os.RemoveAll(dirName)

	cnt := []byte("12345")
	err = ioutil.WriteFile(filepath.Join(dirName, "test1.toml"), cnt, os.ModePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(dirName, "test2.toml"), cnt, os.ModePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(dirName, "test3.toml"), cnt, os.ModePerm)
	require.NoError(t, err)

	secret := []byte("laisky")
	err = AESEncryptFilesInDir(dirName, secret)
	require.NoError(t, err)

	for _, fname := range []string{"test1.enc.toml", "test2.enc.toml", "test3.enc.toml"} {
		fname = filepath.Join(dirName, fname)
		cipher, err := ioutil.ReadFile(fname)
		require.NoError(t, err)

		got, err := DecryptByAes(secret, cipher)
		require.NoError(t, err)

		require.Equal(t, cnt, got)
	}
}

func ExampleAtomicFieldBool() {
	type foo struct {
		v AtomicFieldBool
	}

	f := new(foo)
	f.v.SetTrue()
	fmt.Println(f.v.True())
	// Output: true
}

func TestAtomicFieldBool(t *testing.T) {
	type foo struct {
		v AtomicFieldBool
	}

	f := new(foo)
	require.False(t, f.v.True())

	t.Run("baseline", func(t *testing.T) {
		f.v.SetTrue()
		require.True(t, f.v.True())

		f.v.SetFalse()
		require.False(t, f.v.True())
	})

	t.Run("race", func(t *testing.T) {
		var pool errgroup.Group
		for i := 0; i < 10; i++ {
			pool.Go(func() error {
				rander := rand.New(rand.NewSource(time.Now().Unix()))
				for i := 0; i < 1000; i++ {
					if rander.Intn(10) < 5 {
						f.v.SetTrue()
						f.v.True()
					} else {
						f.v.SetFalse()
						f.v.True()
					}
				}

				return nil
			})
		}

		require.NoError(t, pool.Wait())
	})
}
