package utils

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// St type of project settings
type St struct {
	sync.Mutex
}

// Settings is the settings for this project
var Settings = &St{}

// Reload get setting by key
func (s *St) Reload() {
	s.Lock()
	defer s.Unlock()

	LoadSettings()
}

// Get get setting by key
func (s *St) Get(key string) interface{} {
	s.Lock()
	defer s.Unlock()

	return viper.Get(key)
}

// GetString get setting by key
func (s *St) GetString(key string) string {
	s.Lock()
	defer s.Unlock()

	return viper.GetString(key)
}

// GetBool get setting by key
func (s *St) GetBool(key string) bool {
	s.Lock()
	defer s.Unlock()

	return viper.GetBool(key)
}

// GetInt get setting by key
func (s *St) GetInt(key string) int {
	s.Lock()
	defer s.Unlock()

	return viper.GetInt(key)
}

// GetInt64 get setting by key
func (s *St) GetInt64(key string) int64 {
	s.Lock()
	defer s.Unlock()

	return viper.GetInt64(key)
}

// GetDuration get setting by key
func (s *St) GetDuration(key string) time.Duration {
	s.Lock()
	defer s.Unlock()

	return viper.GetDuration(key)
}

// Set set setting by key
func (s *St) Set(key string, val interface{}) {
	s.Lock()
	defer s.Unlock()

	viper.Set(key, val)
}

// SetupSettings load config file settings.yml
func SetupSettings() {
	viper.SetConfigType("yaml")
	viper.SetConfigName("settings") // name of config file (without extension)
	viper.AddConfigPath("/etc/go-ramjet/settings/")
	viper.AddConfigPath(os.Getenv("GOPATH") + "/src/github.com/Laisky/settings/")
	viper.AddConfigPath(".")

	LoadSettings()
	// WatchSettingsFileChange()
}

// LoadSettings load settings file
func LoadSettings() {
	err := viper.ReadInConfig() // Find and read the config file
	// log.Info("load settings")
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s", err))
	}
}
