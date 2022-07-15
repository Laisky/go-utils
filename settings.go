package utils

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	zap "github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
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

// SettingsType type of project settings
type SettingsType struct {
	sync.RWMutex

	v *viper.Viper

	// watchOnce sync.Once
}

// Settings is the settings for this project
//
// enhance viper.Viper with threadsafe and richer features.
//
// Basic Usage
//
//   import gutils "github.com/Laisky/go-utils/v2"
//
//	 gutils.Settings.
var Settings = NewSettings()

// NewSettings new settings
func NewSettings() *SettingsType {
	return &SettingsType{
		v: viper.New(),
	}
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

// Unmarshal unmarshals the config into a Struct. Make sure that the tags
// on the fields of the structure are properly set.
func (s *SettingsType) Unmarshal(obj interface{}) error {
	s.RLock()
	defer s.RUnlock()

	return viper.Unmarshal(obj)
}

// UnmarshalKey takes a single key and unmarshals it into a Struct.
func (s *SettingsType) UnmarshalKey(key string, obj interface{}) error {
	s.RLock()
	defer s.RUnlock()

	return viper.UnmarshalKey(key, obj)
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

// LoadFromDir load settings from dir, default fname is `settings.yml`
func (s *SettingsType) LoadFromDir(dirPath string) error {
	Logger.Info("Setup settings", zap.String("dirpath", dirPath))
	fpath := filepath.Join(dirPath, defaultConfigFileName)
	return s.LoadFromFile(fpath)
}

type settingsOpt struct {
	enableInclude bool
	aesKey        []byte
	// encryptedSuffix encrypted file must end with this suffix
	encryptedSuffix string
	// watchModify automate update when file modified
	watchModify bool
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
func WithSettingsWatchFileModified() SettingsOptFunc {
	return func(opt *settingsOpt) error {
		opt.watchModify = true
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

// LoadFromFile load settings from file
func (s *SettingsType) LoadFromFile(filePath string, opts ...SettingsOptFunc) (err error) {
	opt, err := new(settingsOpt).fillDefault().applyOptfs()
	if err != nil {
		return errors.Wrap(err, "apply options")
	}

	logger := Logger.With(
		zap.String("file", filePath),
		zap.Bool("include", opt.enableInclude),
	)
	cfgDir := filepath.Dir(filePath)
	cfgFiles := []string{filePath}
	var fp *os.File

RECUR_INCLUDE_LOOP:
	for {
		if fp, err = os.Open(filePath); err != nil {
			return errors.Wrapf(err, "open config file `%s`", filePath)
		}
		defer CloseQuietly(fp)

		viper.SetConfigType(strings.TrimLeft(filepath.Ext(strings.TrimSuffix(filePath, opt.encryptedSuffix)), "."))
		if isSettingsFileEncrypted(opt, filePath) {
			decrptReader, err := NewAesReaderWrapper(fp, opt.aesKey)
			if err != nil {
				return err
			}

			if err = viper.ReadConfig(decrptReader); err != nil {
				return errors.Wrapf(err, "load encrypted config from file `%s`", filePath)
			}
		} else {
			if err = viper.ReadConfig(fp); err != nil {
				return errors.Wrapf(err, "load config from file `%s`", filePath)
			}
		}

		_ = fp.Close()
		if filePath = viper.GetString(settingsIncludeKey); filePath == "" {
			break
		}

		filePath = filepath.Join(cfgDir, filePath)
		for _, f := range cfgFiles {
			if f == filePath {
				break RECUR_INCLUDE_LOOP
			}
		}

		cfgFiles = append(cfgFiles, filePath)
	}

	if err = s.loadConfigFiles(opt, cfgFiles); err != nil {
		return err
	}

	logger.Info("load configs", zap.Strings("config_files", cfgFiles))
	return nil
}

func (s *SettingsType) loadConfigFiles(opt *settingsOpt, cfgFiles []string) (err error) {
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
			defer CloseQuietly(fp)

			if isSettingsFileEncrypted(opt, filePath) {
				encryptedFp, err := NewAesReaderWrapper(fp, opt.aesKey)
				if err != nil {
					return err
				}

				if err = viper.MergeConfig(encryptedFp); err != nil {
					return errors.Wrapf(err, "merge encrypted config file `%s`", filePath)
				}
			} else {
				if err = viper.MergeConfig(fp); err != nil {
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
func (s *SettingsType) LoadFromConfigServer(url, app, profile, label string) (err error) {
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

// LoadFromConfigServerWithRawYaml load configs from config-server
//
// endpoint `{url}/{app}/{profile}/{label}`
//
// load raw yaml content and parse.
func (s *SettingsType) LoadFromConfigServerWithRawYaml(url, app, profile, label, key string) (err error) {
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
		return errors.Errorf("can not load raw cfg with key `%s`", key)
	}
	Logger.Debug("load raw cfg", zap.String("raw", raw))
	viper.SetConfigType("yaml")
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
		panic(errors.Errorf("fatal error config file: %s", err))
	}
}

type settingsAESEncryptOpt struct {
	ext string
	// suffix will append in encrypted file'name after ext as suffix
	suffix string
}

func (o *settingsAESEncryptOpt) fillDefault() {
	// o.ext = ".toml"
	o.suffix = defaultEncryptSuffix
}

// SettingsEncryptOptf options to encrypt files in dir
type SettingsEncryptOptf func(*settingsAESEncryptOpt) error

// AESEncryptFilesInDirFileExt only encrypt files with specific ext
func AESEncryptFilesInDirFileExt(ext string) SettingsEncryptOptf {
	return func(opt *settingsAESEncryptOpt) error {
		if !strings.HasPrefix(ext, ".") {
			return errors.Errorf("ext should start with `.`")
		}

		opt.ext = ext
		return nil
	}
}

// AESEncryptFilesInDirFileSuffix will append to encrypted's filename as suffix
//
//   xxx.toml -> xxx.toml.enc
func AESEncryptFilesInDirFileSuffix(suffix string) SettingsEncryptOptf {
	return func(opt *settingsAESEncryptOpt) error {
		if !strings.HasPrefix(suffix, ".") {
			return errors.Errorf("suffix should start with `.`")
		}

		opt.suffix = suffix
		return nil
	}
}

// AESEncryptFilesInDir encrypt files in dir
//
// will generate new encrypted files with <suffix> after ext
//
//   xxx.toml -> xxx.toml.enc
func AESEncryptFilesInDir(dir string, secret []byte, opts ...SettingsEncryptOptf) (err error) {
	opt := new(settingsAESEncryptOpt)
	opt.fillDefault()
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return err
		}
	}
	logger := Logger.With(
		zap.String("ext", opt.ext),
		zap.String("suffix", opt.suffix),
	)

	fs, err := ListFilesInDir(dir)
	if err != nil {
		return errors.Wrapf(err, "read dir `%s`", dir)
	}

	var pool errgroup.Group
	for _, fname := range fs {
		if !strings.HasSuffix(fname, opt.ext) {
			continue
		}

		fname := fname
		pool.Go(func() (err error) {
			raw, err := ioutil.ReadFile(fname)
			if err != nil {
				return errors.Wrapf(err, "read file `%s`", fname)
			}

			cipher, err := EncryptByAes(secret, raw)
			if err != nil {
				return errors.Wrapf(err, "encrypt")
			}

			outfname := fname + opt.suffix
			if err = ioutil.WriteFile(outfname, cipher, os.ModePerm); err != nil {
				return errors.Wrapf(err, "write file `%s`", outfname)
			}

			logger.Info("encrypt file", zap.String("src", fname), zap.String("out", outfname))
			return nil
		})
	}

	return pool.Wait()
}
