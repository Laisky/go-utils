package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type ConfigSource struct {
	Name   string                 `json:"name"`
	Source map[string]interface{} `json:"source"`
}

type Config struct {
	Name     string          `json:"name"`
	Profiles []string        `json:"profiles"`
	Label    string          `json:"label"`
	Version  string          `json:"version"`
	Sources  []*ConfigSource `json:"propertySources"`
}

// ConfigSrv can load configuration from Spring-Config-Server
type ConfigSrv struct {
	Url, Profile, Label, App string
	Cfg                      *Config
}

// NewConfigSrv create ConfigSrv
func NewConfigSrv(url, profile, label, app string) *ConfigSrv {
	return &ConfigSrv{
		Url:     url,     // config-server api
		Profile: profile, // env
		Label:   label,   // branch
		App:     app,     // app name
		Cfg:     &Config{},
	}
}

func (c *ConfigSrv) Fetch() error {
	url := strings.Join([]string{c.Url, c.App, c.Profile, c.Label}, "/")
	err := RequestJSONWithClient(httpClient, "get", url, &RequestData{}, c.Cfg)
	if err != nil {
		return errors.Wrap(err, "try to get config got error")
	}

	return nil
}

func (c *ConfigSrv) Get(name string) (interface{}, bool) {
	var (
		item string
		val  interface{}
	)
	for _, src := range c.Cfg.Sources {
		for item, val = range src.Source {
			if item == name {
				return val, true
			}
		}
	}

	return nil, false
}

func (c *ConfigSrv) GetString(name string) (string, bool) {
	if val, ok := c.Get(name); ok {
		return val.(string), true
	} else {
		return "", false
	}
}

func (c *ConfigSrv) GetInt(name string) (int, bool) {
	if val, ok := c.Get(name); ok {
		if i, err := strconv.ParseInt(fmt.Sprintf("%v", val), 10, 64); err != nil {
			Logger.Error("try to parse int got error", zap.String("val", fmt.Sprintf("%v", val)))
			return 0, false
		} else {
			return int(i), true
		}
	} else {
		return 0, false
	}
}

func (c *ConfigSrv) GetBool(name string) (bool, bool) {
	if val, ok := c.Get(name); ok {
		if ret, err := strconv.ParseBool(fmt.Sprintf("%v", val)); err != nil {
			Logger.Error("try to parse bool got error", zap.String("val", fmt.Sprintf("%v", val)))
			return false, false
		} else {
			return ret, true
		}
	} else {
		return false, false
	}
}

func (c *ConfigSrv) Map(set func(string, interface{})) {
	var (
		key string
		val interface{}
		src *ConfigSource
	)
	for i := 0; i < len(c.Cfg.Sources); i++ {
		src = c.Cfg.Sources[i]
		for key, val = range src.Source {
			set(key, val)
		}
	}
}
