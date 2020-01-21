package utils

// import (
// 	"github.com/Laisky/go-utils"
// )
//
// utils.Settings.Setup("/etc/go-ramjet/settings")  // load /etc/go-ramjet/settings.yml
// utils.Settings.Get("key")

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	zap "github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const defaultConfigFileName = "settings.yml"

// SettingsType type of project settings
type SettingsType struct {
	sync.RWMutex
	YamlExt string
}

// Settings is the settings for this project
var Settings = &SettingsType{
	YamlExt: "yaml",
}

// BindPFlags bind pflags to settings
func (s *SettingsType) BindPFlags(p *pflag.FlagSet) error {
	return viper.BindPFlags(p)
}

// Get get setting by key
func (s *SettingsType) Get(key string) interface{} {
	s.RLock()
	defer s.RUnlock()

	return viper.Get(key)
}

// GetString get setting by key
func (s *SettingsType) GetString(key string) string {
	s.RLock()
	defer s.RUnlock()

	return viper.GetString(key)
}

// GetStringSlice get setting by key
func (s *SettingsType) GetStringSlice(key string) []string {
	s.RLock()
	defer s.RUnlock()

	return viper.GetStringSlice(key)
}

// GetBool get setting by key
func (s *SettingsType) GetBool(key string) bool {
	s.RLock()
	defer s.RUnlock()

	return viper.GetBool(key)
}

// GetInt get setting by key
func (s *SettingsType) GetInt(key string) int {
	s.RLock()
	defer s.RUnlock()

	return viper.GetInt(key)
}

// GetInt64 get setting by key
func (s *SettingsType) GetInt64(key string) int64 {
	s.RLock()
	defer s.RUnlock()

	return viper.GetInt64(key)
}

// GetDuration get setting by key
func (s *SettingsType) GetDuration(key string) time.Duration {
	s.RLock()
	defer s.RUnlock()

	return viper.GetDuration(key)
}

// Set set setting by key
func (s *SettingsType) Set(key string, val interface{}) {
	s.Lock()
	defer s.Unlock()

	viper.Set(key, val)
}

// IsSet check whether exists
func (s *SettingsType) IsSet(key string) bool {
	s.Lock()
	defer s.Unlock()

	return viper.IsSet(key)
}

// GetStringMap return map contains interface
func (s *SettingsType) GetStringMap(key string) map[string]interface{} {
	s.RLock()
	defer s.RUnlock()

	return viper.GetStringMap(key)
}

// GetStringMapString return map contains strings
func (s *SettingsType) GetStringMapString(key string) map[string]string {
	s.RLock()
	defer s.RUnlock()

	return viper.GetStringMapString(key)
}

// Setup load config file settings.yml
func (s *SettingsType) Setup(configPath string) error {
	return s.SetupFromDir(configPath)
}

// SetupFromDir load settings from dir, default fname is `settings.yml`
func (s *SettingsType) SetupFromDir(dirPath string) error {
	Logger.Info("Setup settings", zap.String("dirpath", dirPath))
	fpath := filepath.Join(dirPath, defaultConfigFileName)
	return s.SetupFromFile(fpath)
}

// SetupFromFile load settings from file
func (s *SettingsType) SetupFromFile(filePath string) error {
	Logger.Info("Setup settings", zap.String("filePath", filePath))
	viper.SetConfigType(Settings.YamlExt)
	fp, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "try to open config file got error")
	}
	defer fp.Close()
	if err = viper.ReadConfig(bufio.NewReader(fp)); err != nil {
		return errors.Wrap(err, "try to load config file got error")
	}

	return nil
}

// SetupFromConfigServer load configs from config-server,
// endpoint `{url}/{app}/{profile}/{label}`
func (s *SettingsType) SetupFromConfigServer(url, app, profile, label string) (err error) {
	Logger.Info("load settings from remote",
		zap.String("url", url),
		zap.String("profile", profile),
		zap.String("label", label),
		zap.String("app", app))

	srv := NewConfigSrv(url, app, profile, label)
	if err = srv.Fetch(); err != nil {
		return errors.Wrap(err, "try to fetch remote config got error")
	}
	srv.Map(viper.Set)

	return nil
}

// SetupFromConfigServerWithRawYaml load configs from config-server
//
// endpoint `{url}/{app}/{profile}/{label}`
//
// load raw yaml content and parse.
func (s *SettingsType) SetupFromConfigServerWithRawYaml(url, app, profile, label, key string) (err error) {
	Logger.Info("load settings from remote",
		zap.String("url", url),
		zap.String("profile", profile),
		zap.String("label", label),
		zap.String("app", app))

	srv := NewConfigSrv(url, app, profile, label)
	if err = srv.Fetch(); err != nil {
		return errors.Wrap(err, "try to fetch remote config got error")
	}
	raw, ok := srv.GetString(key)
	if !ok {
		return fmt.Errorf("can not load raw cfg with key `%v`", key)
	}
	Logger.Debug("load raw cfg", zap.String("raw", raw))
	viper.SetConfigType(Settings.YamlExt)
	if err = viper.ReadConfig(bytes.NewReader([]byte(raw))); err != nil {
		return errors.Wrap(err, "try to load config file got error")
	}

	return nil
}

// LoadSettings load settings file
func (s *SettingsType) LoadSettings() {
	s.RLock()
	defer s.RUnlock()

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s", err))
	}
}
