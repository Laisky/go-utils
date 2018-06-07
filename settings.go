package utils

// import (
// 	"github.com/Laisky/go-utils"
// )
//
// utils.Settings.Setup("/etc/go-ramjet/settings")  // load /etc/go-ramjet/settings.yml
// utils.Settings.Get("key")

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// SettingsType type of project settings
type SettingsType struct {
	sync.Mutex
}

// Settings is the settings for this project
var Settings = &SettingsType{}

// BindPFlags bind pflags to settings
func (s *SettingsType) BindPFlags(p *pflag.FlagSet) {
	viper.BindPFlags(p)
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

// Setup load config file settings.yml
func (s *SettingsType) Setup(configPath string) {
	viper.SetConfigType("yaml")
	viper.SetConfigName("settings") // name of config file (without extension)
	viper.AddConfigPath(configPath)
	viper.AddConfigPath(".")

	s.LoadSettings()
	// WatchSettingsFileChange()
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
