package config

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/go-utils/v2/encrypt"
	"github.com/Laisky/go-utils/v2/log"
	zap "github.com/Laisky/zap"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// AtomicFieldBool is a bool field which is goroutine-safe
type AtomicFieldBool struct {
	v int64
}

// True value == true
func (a *AtomicFieldBool) True() bool {
	return atomic.LoadInt64(&a.v) == 1
}

// SetTrue set true
func (a *AtomicFieldBool) SetTrue() {
	atomic.StoreInt64(&a.v, 1)
}

// SetFalse set false
func (a *AtomicFieldBool) SetFalse() {
	atomic.StoreInt64(&a.v, 0)
}

const defaultConfigFileName = "settings.yml"

// config type of project settings
type config struct {
	sync.RWMutex

	v *viper.Viper

	watchOnce sync.Once
}

// Shared is the settings for this project
//
// enhance viper.Viper with threadsafe and richer features.
//
// Basic Usage
//
//   import gutils "github.com/Laisky/go-utils/v2"
//
//	 gutils.Shared.
var Shared = New()

var S = Shared

// New new settings
func New() *config {
	return &config{
		v: viper.New(),
	}
}

// BindPFlags bind pflags to settings
func (s *config) BindPFlags(p *pflag.FlagSet) error {
	return s.v.BindPFlags(p)
}

// Get get setting by key
func (s *config) Get(key string) interface{} {
	s.RLock()
	defer s.RUnlock()

	return s.v.Get(key)
}

// GetString get setting by key
func (s *config) GetString(key string) string {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetString(key)
}

// GetStringSlice get setting by key
func (s *config) GetStringSlice(key string) []string {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetStringSlice(key)
}

// GetBool get setting by key
func (s *config) GetBool(key string) bool {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetBool(key)
}

// GetInt get setting by key
func (s *config) GetInt(key string) int {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetInt(key)
}

// GetInt64 get setting by key
func (s *config) GetInt64(key string) int64 {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetInt64(key)
}

// GetDuration get setting by key
func (s *config) GetDuration(key string) time.Duration {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetDuration(key)
}

// Set set setting by key
func (s *config) Set(key string, val interface{}) {
	s.Lock()
	defer s.Unlock()

	s.v.Set(key, val)
}

// IsSet check whether exists
func (s *config) IsSet(key string) bool {
	s.Lock()
	defer s.Unlock()

	return s.v.IsSet(key)
}

// Unmarshal unmarshals the config into a Struct. Make sure that the tags
// on the fields of the structure are properly set.
func (s *config) Unmarshal(obj interface{}) error {
	s.RLock()
	defer s.RUnlock()

	return s.v.Unmarshal(obj)
}

// UnmarshalKey takes a single key and unmarshals it into a Struct.
func (s *config) UnmarshalKey(key string, obj interface{}) error {
	s.RLock()
	defer s.RUnlock()

	return s.v.UnmarshalKey(key, obj)
}

// GetStringMap return map contains interface
func (s *config) GetStringMap(key string) map[string]interface{} {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetStringMap(key)
}

// GetStringMapString return map contains strings
func (s *config) GetStringMapString(key string) map[string]string {
	s.RLock()
	defer s.RUnlock()

	return s.v.GetStringMapString(key)
}

func (s *config) ReadConfig(in io.Reader) error {
	s.Lock()
	defer s.Unlock()

	return s.v.ReadConfig(in)
}

func (s *config) MergeConfig(in io.Reader) error {
	s.Lock()
	defer s.Unlock()

	return s.v.MergeConfig(in)
}

// LoadFromDir load settings from dir, default fname is `settings.yml`
func (s *config) LoadFromDir(dirPath string, opts ...SettingsOptFunc) error {
	fpath := filepath.Join(dirPath, defaultConfigFileName)
	return s.LoadFromFile(fpath, opts...)
}

type settingsOpt struct {
	enableInclude bool
	aesKey        []byte
	// encryptedSuffix encrypted file must end with this suffix
	encryptedSuffix string
	// watchModify automate update when file modified
	watchModify         bool
	watchModifyCallback func(fsnotify.Event)
}

const (
	defaultEncryptSuffix = ".enc"
)

func (o *settingsOpt) fillDefault() *settingsOpt {
	o.encryptedSuffix = defaultEncryptSuffix
	return o
}

func (o *settingsOpt) applyOptfs(opts ...SettingsOptFunc) (*settingsOpt, error) {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// SettingsOptFunc opt for settings
type SettingsOptFunc func(*settingsOpt) error

// WithSettingsEnableInclude enable `include` in config file
func WithSettingsEnableInclude() SettingsOptFunc {
	return func(opt *settingsOpt) error {
		opt.enableInclude = true
		return nil
	}
}

// WithSettingsAesEncrypt decrypt config file by aes
func WithSettingsAesEncrypt(key []byte) SettingsOptFunc {
	return func(opt *settingsOpt) error {
		if len(key) == 0 {
			return errors.Errorf("aes key is empty")
		}

		opt.aesKey = key
		return nil
	}
}

// WithSettingsEncryptedFileSuffix only decrypt files which name ends with `encryptedSuffix`
func WithSettingsEncryptedFileSuffix(suffix string) SettingsOptFunc {
	return func(opt *settingsOpt) error {
		opt.encryptedSuffix = suffix
		return nil
	}
}

// WithSettingsWatchFileModified automate update when file modified
//
// callback will be called when file modified.
// you can set callback to nil if you don't want to process file changing event manually.
func WithSettingsWatchFileModified(callback func(fsnotify.Event)) SettingsOptFunc {
	return func(opt *settingsOpt) error {
		opt.watchModify = true
		opt.watchModifyCallback = callback
		return nil
	}
}

const settingsIncludeKey = "include"

// isSettingsFileEncrypted encrypted file's name contains encryptedMark
func isSettingsFileEncrypted(opt *settingsOpt, fname string) bool {
	if opt.aesKey == nil {
		return false
	}

	if opt.encryptedSuffix != "" &&
		strings.HasSuffix(fname, opt.encryptedSuffix) {
		return true
	}

	return false
}

func (s *config) watch(opt *settingsOpt, entryFile string, files []string, opts ...SettingsOptFunc) {
	s.watchOnce.Do(func() {
		if err := gutils.WatchFileChanging(context.Background(), files, func(e fsnotify.Event) {
			if err := s.LoadFromFile(entryFile, opts...); err != nil {
				log.Shared.Error("file watcher auto reload settings", zap.Error(err))
			}

			if opt.watchModifyCallback != nil {
				opt.watchModifyCallback(e)
			}
		}); err != nil {
			log.Shared.Error("watch file error", zap.Error(err), zap.Strings("files", files))
		}

		log.Shared.Debug("watching config files", zap.Strings("files", files))
	})
}

// LoadFromFile load settings from file
func (s *config) LoadFromFile(entryFile string, opts ...SettingsOptFunc) (err error) {
	opt, err := new(settingsOpt).fillDefault().applyOptfs(opts...)
	if err != nil {
		return errors.Wrap(err, "apply options")
	}

	logger := log.Shared.With(
		zap.String("file", entryFile),
		zap.Bool("include", opt.enableInclude),
	)

	curFpath := entryFile
	cfgDir := filepath.Dir(entryFile)
	cfgFiles := []string{entryFile}
	var fp *os.File

RECUR_INCLUDE_LOOP:
	for {
		if fp, err = os.Open(curFpath); err != nil {
			return errors.Wrapf(err, "open config file `%s`", curFpath)
		}
		defer gutils.CloseQuietly(fp)

		s.v.SetConfigType(strings.TrimLeft(filepath.Ext(strings.TrimSuffix(curFpath, opt.encryptedSuffix)), "."))
		if isSettingsFileEncrypted(opt, curFpath) {
			decrptReader, err := encrypt.NewAesReaderWrapper(fp, opt.aesKey)
			if err != nil {
				return err
			}

			if err = s.ReadConfig(decrptReader); err != nil {
				return errors.Wrapf(err, "load encrypted config from file `%s`", curFpath)
			}
		} else {
			if err = s.ReadConfig(fp); err != nil {
				return errors.Wrapf(err, "load config from file `%s`", curFpath)
			}
		}

		_ = fp.Close()
		if curFpath = s.GetString(settingsIncludeKey); curFpath == "" {
			break
		}

		curFpath = filepath.Join(cfgDir, curFpath)
		for _, f := range cfgFiles {
			if f == curFpath {
				break RECUR_INCLUDE_LOOP
			}
		}

		cfgFiles = append(cfgFiles, curFpath)
	}

	if err = s.loadConfigFiles(opt, cfgFiles); err != nil {
		return err
	}

	if opt.watchModify {
		s.watch(opt, entryFile, cfgFiles, opts...)
	}

	logger.Info("load configs", zap.Strings("config_files", cfgFiles))
	return nil
}

func (s *config) loadConfigFiles(opt *settingsOpt, cfgFiles []string) (err error) {
	var (
		filePath string
		fp       *os.File
	)
	for i := len(cfgFiles) - 1; i >= 0; i-- {
		if err = func() error {
			filePath = cfgFiles[i]
			if fp, err = os.Open(filePath); err != nil {
				return errors.Wrapf(err, "open config file `%s`", filePath)
			}
			defer gutils.CloseQuietly(fp)

			if isSettingsFileEncrypted(opt, filePath) {
				encryptedFp, err := encrypt.NewAesReaderWrapper(fp, opt.aesKey)
				if err != nil {
					return err
				}

				if err = s.MergeConfig(encryptedFp); err != nil {
					return errors.Wrapf(err, "merge encrypted config file `%s`", filePath)
				}
			} else {
				if err = s.MergeConfig(fp); err != nil {
					return errors.Wrapf(err, "merge config file `%s`", filePath)
				}
			}

			return nil
		}(); err != nil {
			return err
		}
	}

	return nil
}

// LoadFromConfigServer load configs from config-server,
//
// endpoint `{url}/{app}/{profile}/{label}`
func (s *config) LoadFromConfigServer(url, app, profile, label string) (err error) {
	log.Shared.Info("load settings from remote",
		zap.String("url", url),
		zap.String("profile", profile),
		zap.String("label", label),
		zap.String("app", app))

	srv := NewConfigSrv(url, app, profile, label)
	if err = srv.Fetch(); err != nil {
		return errors.Wrap(err, "try to fetch remote config got error")
	}
	srv.Map(s.v.Set)

	return nil
}

// LoadFromConfigServerWithRawYaml load configs from config-server
//
// endpoint `{url}/{app}/{profile}/{label}`
//
// load raw yaml content and parse.
func (s *config) LoadFromConfigServerWithRawYaml(url, app, profile, label, key string) (err error) {
	log.Shared.Info("load settings from remote",
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
		return errors.Errorf("can not load raw cfg with key `%s`", key)
	}
	log.Shared.Debug("load raw cfg", zap.String("raw", raw))
	s.v.SetConfigType("yaml")
	if err = s.v.ReadConfig(bytes.NewReader([]byte(raw))); err != nil {
		return errors.Wrap(err, "try to load config file got error")
	}

	return nil
}

// LoadSettings load settings file
func (s *config) LoadSettings() {
	s.RLock()
	defer s.RUnlock()

	err := s.v.ReadInConfig() // Find and read the config file
	if err != nil {           // Handle errors reading the config file
		panic(errors.Errorf("fatal error config file: %s", err))
	}
}
