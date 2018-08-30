package utils

// import (
// 	"github.com/Laisky/go-utils"
// )
//
// utils.Settings.Setup("/etc/go-ramjet/settings")  // load /etc/go-ramjet/settings.yml
// utils.Settings.Get("key")

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	zap "go.uber.org/zap"
)

// SettingsType type of project settings
type SettingsType struct {
	sync.Mutex
}

// Settings is the settings for this project
var Settings = &SettingsType{}

// BindPFlags bind pflags to settings
func (s *SettingsType) BindPFlags(p *pflag.FlagSet) error {
	return viper.BindPFlags(p)
}

// Get get setting by key
func (s *SettingsType) Get(key string) interface{} {
	s.Lock()
	defer s.Unlock()

	return viper.Get(key)
}

// GetString get setting by key
func (s *SettingsType) GetString(key string) string {
	s.Lock()
	defer s.Unlock()

	return viper.GetString(key)
}

// GetStringSlice get setting by key
func (s *SettingsType) GetStringSlice(key string) []string {
	s.Lock()
	defer s.Unlock()

	return viper.GetStringSlice(key)
}

// GetBool get setting by key
func (s *SettingsType) GetBool(key string) bool {
	s.Lock()
	defer s.Unlock()

	return viper.GetBool(key)
}

// GetInt get setting by key
func (s *SettingsType) GetInt(key string) int {
	s.Lock()
	defer s.Unlock()

	return viper.GetInt(key)
}

// GetInt64 get setting by key
func (s *SettingsType) GetInt64(key string) int64 {
	s.Lock()
	defer s.Unlock()

	return viper.GetInt64(key)
}

// GetDuration get setting by key
func (s *SettingsType) GetDuration(key string) time.Duration {
	s.Lock()
	defer s.Unlock()

	return viper.GetDuration(key)
}

// Set set setting by key
func (s *SettingsType) Set(key string, val interface{}) {
	s.Lock()
	defer s.Unlock()

	viper.Set(key, val)
}

const CFG_FNAME = "settings.yml"

// Setup load config file settings.yml
func (s *SettingsType) Setup(configPath string) error {
	fpath := filepath.Join(configPath, CFG_FNAME)
	Logger.Info("Setup settings", zap.String("path", fpath))
	viper.SetConfigType("yaml")
	fp, err := os.Open(fpath)
	if err != nil {
		return errors.Wrap(err, "try to open config file got error")
	}
	defer fp.Close()
	if err = viper.ReadConfig(bufio.NewReader(fp)); err != nil {
		return errors.Wrap(err, "try to load config file got error")
	}

	// `--remote-config=true` enable remote config
	if s.GetBool("remote-config") {
		Logger.Info("load settings from remote",
			zap.String("url", s.GetString("config.url")),
			zap.String("profile", s.GetString("config.profile")),
			zap.String("label", s.GetString("config.label")),
			zap.String("app", s.GetString("config.app")))
		cfg := NewConfigSrv(s.GetString("config.url"),
			s.GetString("config.profile"),
			s.GetString("config.label"),
			s.GetString("config.app"))
		cfg.Map(viper.Set)
	}

	return nil
}

// LoadSettings load settings file
func (s *SettingsType) LoadSettings() {
	s.Lock()
	defer s.Unlock()

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s", err))
	}
}
