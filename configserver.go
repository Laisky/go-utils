package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

// ConfigSource config item in config-server
type ConfigSource struct {
	Name   string                 `json:"name"`
	Source map[string]interface{} `json:"source"`
}

// Config whole configuation return by config-server
type Config struct {
	Name     string          `json:"name"`
	Profiles []string        `json:"profiles"`
	Label    string          `json:"label"`
	Version  string          `json:"version"`
	Sources  []*ConfigSource `json:"propertySources"`
}

// ConfigSrv can load configuration from Spring-Cloud-Config-Server
type ConfigSrv struct {
	RemoteCfg *Config

	url, // config-server api
	profile, // env
	label, // branch
	app string // app name
}

// NewConfigSrv create ConfigSrv
func NewConfigSrv(url, app, profile, label string) *ConfigSrv {
	return &ConfigSrv{
		RemoteCfg: &Config{},
		url:       url,
		app:       app,
		label:     label,
		profile:   profile,
	}
}

// Fetch load data from config-server
func (c *ConfigSrv) Fetch() error {
	url := strings.Join([]string{c.url, c.app, c.profile, c.label}, "/")
	err := RequestJSONWithClient(httpClient, "get", url, &RequestData{}, c.RemoteCfg)
	if err != nil {
		return errors.Wrap(err, "try to get config got error")
	}

	return nil
}

// Get get `interface{}` from the localcache of config-server
func (c *ConfigSrv) Get(name string) (interface{}, bool) {
	var (
		item string
		val  interface{}
	)
	for _, src := range c.RemoteCfg.Sources {
		for item, val = range src.Source {
			if item == name {
				return val, true
			}
		}
	}

	return nil, false
}

// GetString get `string` from the localcache of config-server
func (c *ConfigSrv) GetString(name string) (string, bool) {
	if val, ok := c.Get(name); ok {
		return val.(string), true
	}

	return "", false
}

// GetInt get `int` from the localcache of config-server
func (c *ConfigSrv) GetInt(name string) (val int, ok bool) {
	var (
		itf   interface{}
		val64 int64
		err   error
	)
	if itf, ok = c.Get(name); ok {
		switch v := itf.(type) {
		case int:
			val = v
		case int64:
			val = int(v)
		case string:
			if val64, err = strconv.ParseInt(v, 10, 64); err != nil {
				Logger.Error("cannot parse string to int64",
					zap.String("name", name),
					zap.String("val", fmt.Sprint(v)))
				return val, false
			}
			val = int(val64)
		default:
			Logger.Error("unknown type",
				zap.String("name", name),
				zap.String("val", fmt.Sprint(v)))
			return val, false
		}

		return
	}

	return
}

// GetBool get `bool` from the localcache of config-server
func (c *ConfigSrv) GetBool(name string) (val bool, ok bool) {
	var (
		itf interface{}
		err error
	)
	if itf, ok = c.Get(name); ok {
		switch v := itf.(type) {
		case int:
			val = v != 0
		case int64:
			val = v != 0
		case string:
			if val, err = strconv.ParseBool(v); err != nil {
				Logger.Error("cannot parse string to bool",
					zap.String("name", name),
					zap.String("val", fmt.Sprint(v)))
				return val, false
			}
		default:
			Logger.Error("unknown type",
				zap.String("name", name),
				zap.String("val", fmt.Sprint(v)))
			return val, false
		}

		return
	}

	return
}

// Map interate `set(k, v)`
func (c *ConfigSrv) Map(set func(string, interface{})) {
	var (
		key string
		val interface{}
		src *ConfigSource
	)
	for i := 0; i < len(c.RemoteCfg.Sources); i++ {
		src = c.RemoteCfg.Sources[i]
		for key, val = range src.Source {
			Logger.Debug("set settings", zap.String("key", key), zap.String("val", fmt.Sprint(val)))
			set(key, val)
		}
	}
}
