package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
)

var jsonArg struct {
	Recursive bool
	Exts      string
	Dry       bool
	Sort      string
}

func init() {
	rootCmd.AddCommand(jsonCmd)
	jsonCmd.AddCommand(jsonSortCmd)

	jsonSortCmd.Flags().BoolVarP(&jsonArg.Recursive, "recursive", "r", false, "recursively find json files")
	jsonSortCmd.Flags().StringVar(&jsonArg.Exts, "ext", ".json", "supported file name suffixes as a list, split by comma")
	jsonSortCmd.Flags().BoolVar(&jsonArg.Dry, "dry", false, "only list files, do not perform sorting")
	jsonSortCmd.Flags().StringVar(&jsonArg.Sort, "sort", "asc", "ascending or descending order (asc|desc)")
}

// jsonCmd json tools
var jsonCmd = &cobra.Command{
	Use:   "json",
	Short: "json tools",
	Long:  `json tools`,
	Args:  NoExtraArgs,
}

// jsonSortCmd sort json files by keys
var jsonSortCmd = &cobra.Command{
	Use:   "sort",
	Short: "sort json files by keys",
	Long: gutils.Dedent(`
		Sort json files by keys recursively.

		Example:
			gutils json sort -r --ext=".json,.jsonx" --sort=desc .
	`),
	Args: cobra.MinimumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		if jsonArg.Sort != "asc" && jsonArg.Sort != "desc" {
			glog.Shared.Panic("sort must be asc or desc")
		}

		exts := strings.Split(jsonArg.Exts, ",")
		for i := range exts {
			exts[i] = strings.TrimSpace(exts[i])
		}

		for _, path := range args {
			if err := sortJSONPath(path, exts, jsonArg.Recursive, jsonArg.Dry, jsonArg.Sort == "desc"); err != nil {
				glog.Shared.Panic("sort json", zap.String("path", path), zap.Error(err))
			}
		}
	},
}

// sortJSONPath find and sort json files in path
//
// Parameters:
//   - path: path to find json files
//   - exts: supported file name suffixes
//   - recursive: recursively find json files
//   - dry: only list files, do not perform sorting
//   - desc: descending order
//
// Returns:
//   - error: error if any
func sortJSONPath(path string, exts []string, recursive, dry, desc bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "stat %q", path)
	}

	if !info.IsDir() {
		return sortJSONFile(path, dry, desc)
	}

	var files []string
	if recursive {
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			for _, ext := range exts {
				if strings.HasSuffix(p, ext) {
					files = append(files, p)
					break
				}
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return errors.Wrapf(err, "read dir %q", path)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			for _, ext := range exts {
				if strings.HasSuffix(entry.Name(), ext) {
					files = append(files, filepath.Join(path, entry.Name()))
					break
				}
			}
		}
	}

	if err != nil {
		return errors.Wrapf(err, "list files in %q", path)
	}

	for _, f := range files {
		if err := sortJSONFile(f, dry, desc); err != nil {
			return err
		}
	}

	return nil
}

// sortJSONFile sort a single json file
//
// Parameters:
//   - fpath: file path
//   - dry: only list files, do not perform sorting
//   - desc: descending order
//
// Returns:
//   - error: error if any
func sortJSONFile(fpath string, dry, desc bool) error {
	if dry {
		fmt.Printf("found json file: %s\n", fpath)
		return nil
	}

	glog.Shared.Info("sorting json file", zap.String("file", fpath))
	raw, err := os.ReadFile(fpath)
	if err != nil {
		return errors.Wrapf(err, "read file %q", fpath)
	}

	var data interface{}
	if err = json.Unmarshal(raw, &data); err != nil {
		return errors.Wrapf(err, "unmarshal json %q", fpath)
	}

	sortedData := sortRecursive(data, desc)
	out, err := json.MarshalIndent(sortedData, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "marshal sorted json %q", fpath)
	}
	out = append(out, '\n')

	if err = os.WriteFile(fpath, out, 0644); err != nil {
		return errors.Wrapf(err, "write file %q", fpath)
	}

	return nil
}

// sortedMap is a helper to marshal map with sorted keys
type sortedMap struct {
	keys []string
	data map[string]interface{}
	desc bool
}

// MarshalJSON implements json.Marshaler
func (m sortedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	sort.Slice(m.keys, func(i, j int) bool {
		if m.desc {
			return m.keys[i] > m.keys[j]
		}
		return m.keys[i] < m.keys[j]
	})

	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyByte, _ := json.Marshal(k)
		buf.Write(keyByte)
		buf.WriteByte(':')
		valByte, err := json.Marshal(m.data[k])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		buf.Write(valByte)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// sortRecursive recursively wrap map into sortedMap
//
// Parameters:
//   - data: json data
//   - desc: descending order
//
// Returns:
//   - interface{}: sorted json data
func sortRecursive(data interface{}, desc bool) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		sm := sortedMap{
			data: make(map[string]interface{}),
			desc: desc,
		}
		for k, val := range v {
			sm.keys = append(sm.keys, k)
			sm.data[k] = sortRecursive(val, desc)
		}
		return sm
	case []interface{}:
		for i, val := range v {
			v[i] = sortRecursive(val, desc)
		}
		return v
	default:
		return v
	}
}
