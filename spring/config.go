package spring

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var (
	httpClient = &http.Client{ // default http client
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 20,
		},
		Timeout: time.Duration(30) * time.Second,
	}
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
	err := utils.RequestJSONWithClient(httpClient, "get", url, &utils.RequestData{}, c.Cfg)
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
			utils.Logger.Error("try to parse int got error", zap.String("val", fmt.Sprintf("%v", val)))
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
			utils.Logger.Error("try to parse bool got error", zap.String("val", fmt.Sprintf("%v", val)))
			return false, false
		} else {
			return ret, true
		}
	} else {
		return false, false
	}
}
