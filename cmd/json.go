package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/internal/fileguard"
	glog "github.com/Laisky/go-utils/v6/log"
)

var jsonArg struct {
	Recursive   bool
	Exts        string
	Dry         bool
	Sort        string
	Indent      int
	Insensitive bool
}

// init registers the "json" command on the root command together with its "sort" subcommand, and binds the
// sort flags --recursive/-r, --ext, --dry, --sort (asc|desc), --indent and --insensitive/-i to jsonArg.
func init() {
	rootCmd.AddCommand(jsonCmd)
	jsonCmd.AddCommand(jsonSortCmd)

	jsonSortCmd.Flags().BoolVarP(&jsonArg.Recursive,
		"recursive", "r", false, "recursively find json files")
	jsonSortCmd.Flags().StringVar(&jsonArg.Exts, "ext",
		".json", "supported file name suffixes as a list, split by comma")
	jsonSortCmd.Flags().BoolVar(&jsonArg.Dry, "dry", false,
		"only list files, do not perform sorting")
	jsonSortCmd.Flags().StringVar(&jsonArg.Sort, "sort", "asc",
		"ascending or descending order (asc|desc)")
	jsonSortCmd.Flags().IntVar(&jsonArg.Indent, "indent", 2, "indent blanks")
	jsonSortCmd.Flags().BoolVarP(&jsonArg.Insensitive,
		"insensitive", "i", true, "case-insensitive sorting")
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
			if err := sortJSONPath(
				path,
				exts,
				jsonArg.Recursive,
				jsonArg.Dry,
				jsonArg.Sort == "desc",
				jsonArg.Indent,
				jsonArg.Insensitive,
			); err != nil {
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
//   - indent: indent blanks
//   - insensitive: case-insensitive sorting
//
// Returns:
//   - error: error if any
func sortJSONPath(path string, exts []string, recursive, dry, desc bool, indent int, insensitive bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "stat %q", path)
	}

	if !info.IsDir() {
		return sortJSONFile(path, dry, desc, indent, insensitive)
	}

	var files []string
	if recursive {
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if isJSONCandidate(p, p, d, exts) {
				files = append(files, p)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return errors.Wrapf(err, "read dir %q", path)
		}
		for _, entry := range entries {
			fpath := filepath.Join(path, entry.Name())
			if isJSONCandidate(fpath, entry.Name(), entry, exts) {
				files = append(files, fpath)
			}
		}
	}

	if err != nil {
		return errors.Wrapf(err, "list files in %q", path)
	}

	for _, f := range files {
		if err := sortJSONFile(f, dry, desc, indent, insensitive); err != nil {
			return err
		}
	}

	return nil
}

// isJSONCandidate reports whether a scanned directory entry should be sorted.
// Directories are skipped silently; links and other non-regular entries are
// skipped with a debug log so that a scan never rewrites a link target. It takes
// the entry's full path for logging, the string matched against the suffixes, the
// entry and the accepted suffixes, and returns true for a matching regular file.
func isJSONCandidate(fpath, match string, entry os.DirEntry, exts []string) bool {
	if entry.IsDir() {
		return false
	}
	if !entry.Type().IsRegular() {
		glog.Shared.Debug("skip non-regular json candidate", zap.String("file", fpath))
		return false
	}
	for _, ext := range exts {
		if strings.HasSuffix(match, ext) {
			return true
		}
	}
	return false
}

// sortJSONFile sort a single json file
//
// Only regular files are rewritten; a symbolic link, directory or special file is
// rejected so that the rewrite can never modify an unrelated link target. The
// file is read through a descriptor bound to the inspected inode, and the sorted
// output is published with an atomic replace that keeps the original permission
// bits. The replacement is a new inode owned by the current user.
//
// Parameters:
//   - fpath: file path
//   - dry: only list files, do not perform sorting
//   - desc: descending order
//   - indent: indent blanks
//   - insensitive: case-insensitive sorting
//
// Returns:
//   - error: error if any
func sortJSONFile(fpath string, dry, desc bool, indent int, insensitive bool) error {
	info, err := os.Lstat(fpath)
	if err != nil {
		return errors.Wrapf(err, "inspect json file %q", fpath)
	}
	if !info.Mode().IsRegular() {
		return errors.Wrapf(fileguard.ErrNotRegular, "refuse to rewrite json file %q of type %s",
			fpath, info.Mode().Type())
	}

	if dry {
		fmt.Printf("found json file: %s\n", fpath)
		return nil
	}

	glog.Shared.Info("sorting json file", zap.String("file", fpath))
	raw, err := readInspectedRegularFile(fpath, info)
	if err != nil {
		return errors.Wrapf(err, "read file %q", fpath)
	}

	var data interface{}
	if err = json.Unmarshal(raw, &data); err != nil {
		return errors.Wrapf(err, "unmarshal json %q", fpath)
	}

	sortedData := sortRecursive(data, desc, insensitive)
	out, err := json.MarshalIndent(sortedData, "", strings.Repeat(" ", indent))
	if err != nil {
		return errors.Wrapf(err, "marshal sorted json %q", fpath)
	}
	out = append(out, '\n')

	// Publish through a private temporary file and rename: the destination entry
	// is replaced, never written through, and the original mode is restored
	// explicitly because the creation mode is reduced by the umask.
	perm := info.Mode().Perm()
	if err = fileguard.Replace(fpath, 0, perm, func(fp *os.File) error {
		if err := fp.Chmod(perm); err != nil {
			return errors.Wrap(err, "restore json file mode")
		}
		if _, err := fp.Write(out); err != nil {
			return errors.Wrap(err, "write sorted json")
		}
		return nil
	}); err != nil {
		return errors.Wrapf(err, "rewrite file %q", fpath)
	}

	return nil
}

// readInspectedRegularFile reads fpath only if the opened descriptor is still the
// regular file described by info, so a link swapped in after inspection cannot
// redirect the read. It takes the path and its Lstat metadata and returns the
// file content or an error.
func readInspectedRegularFile(fpath string, info os.FileInfo) (data []byte, retErr error) {
	root, err := os.OpenRoot(filepath.Dir(fpath))
	if err != nil {
		return nil, errors.Wrap(err, "open json directory")
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close json directory"))
		}
	}()
	fp, err := fileguard.OpenRegular(root, filepath.Base(fpath), info)
	if err != nil {
		return nil, errors.Wrap(err, "open inspected json file")
	}
	defer func() {
		if err := fp.Close(); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "close json file"))
		}
	}()
	data, err = io.ReadAll(fp)
	if err != nil {
		return nil, errors.Wrap(err, "read json file")
	}
	return data, nil
}

// sortedMap is a helper to marshal map with sorted keys
type sortedMap struct {
	keys        []string
	data        map[string]interface{}
	desc        bool
	insensitive bool
}

// MarshalJSON implements json.Marshaler
func (m sortedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	sort.Slice(m.keys, func(i, j int) bool {
		ki, kj := m.keys[i], m.keys[j]
		if m.insensitive {
			ki = strings.ToLower(ki)
			kj = strings.ToLower(kj)
		}

		var res bool
		if ki != kj {
			if m.desc {
				res = ki > kj
			} else {
				res = ki < kj
			}
		} else {
			// fallback to case-sensitive if insensitive keys are equal
			if m.desc {
				res = m.keys[i] > m.keys[j]
			} else {
				res = m.keys[i] < m.keys[j]
			}
		}

		glog.Shared.Debug("compare json keys",
			zap.String("key_i", m.keys[i]),
			zap.String("key_j", m.keys[j]),
			zap.Bool("insensitive", m.insensitive),
			zap.Bool("result", res))
		return res
	})

	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyByte, err := json.Marshal(k)
		if err != nil {
			return nil, errors.Wrapf(err, "marshal key %q", k)
		}
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
//   - insensitive: case-insensitive sorting
//
// Returns:
//   - interface{}: sorted json data
func sortRecursive(data interface{}, desc bool, insensitive bool) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		sm := sortedMap{
			data:        make(map[string]interface{}),
			desc:        desc,
			insensitive: insensitive,
		}
		for k, val := range v {
			sm.keys = append(sm.keys, k)
			sm.data[k] = sortRecursive(val, desc, insensitive)
		}
		return sm
	case []interface{}:
		for i, val := range v {
			v[i] = sortRecursive(val, desc, insensitive)
		}
		return v
	default:
		return v
	}
}
