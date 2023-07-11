package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v4"
	glog "github.com/Laisky/go-utils/v4/log"
)

func init() {
	rootCmd.AddCommand(renameCMD)
	renameAvCmd.Flags().StringVarP(&renameAvCmdArgs.dir,
		"dir", "d", "", "directory")
	renameAvCmd.Flags().BoolVar(&renameAvCmdArgs.dry,
		"dry", false, "dry run")
	renameAvCmd.Flags().StringSliceVarP(&renameAvCmdArgs.exts,
		"exts", "e", []string{".mp4", ".avi", ".mov"}, "files with these exts will be processed")
}

var renameCMD = &cobra.Command{
	Use:   "rename",
	Short: "rename",
	Long: gutils.Dedent(`
		rename files in directory
	`),
	Args: NoExtraArgs,
}

var renameAvCmdArgs = struct {
	dir     string
	dry     bool
	recurse bool
	exts    []string
}{}

var renameAvCmd = &cobra.Command{
	Use:   "av",
	Short: "av",
	Long: gutils.Dedent(`
		rename all av files to standard format

		Examples:
			$ gutils rename av -d /path/to/dir
	`),
	Args: NoExtraArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := []gutils.ListFilesInDirOptionFunc{}
		if renameAvCmdArgs.recurse {
			opts = append(opts, gutils.ListFilesInDirRecursive())
		}

		opts = append(opts, gutils.ListFilesInDirFilter(
			func(fname string) bool {
				if len(renameAvCmdArgs.exts) == 0 {
					return true
				}

				for _, ext := range renameAvCmdArgs.exts {
					if strings.HasSuffix(fname, ext) {
						return true
					}
				}

				return false
			}),
		)

		fs, err := gutils.ListFilesInDir(renameAvCmdArgs.dir, opts...)
		if err != nil {
			return errors.Wrapf(err, "list files in dir %v", renameAvCmdArgs.dir)
		}

		for _, f := range fs {
			target := convertAvFilename(f)
			if target == f {
				continue
			}

			if renameAvCmdArgs.dry {
				glog.Shared.Info("rename", zap.String("from", f), zap.String("to", target))
				continue
			}

			if err = os.Rename(f, target); err != nil {
				return errors.Wrapf(err, "rename %v -> %v", f, target)
			}

			glog.Shared.Info("rename", zap.String("from", f), zap.String("to", target))
		}

		return nil
	},
}

var convertAvFnameRegexp = regexp.MustCompile(`(?P<name>(?:FC2-)?[a-zA-Z]+(?:[\-_]hd)?[\-_]\d+(?:[\-_]\d)?)`)

// convertAvFilename convert common AV filenames to a standard format
//
// For example, the filename "SSIS-448-C.mp4", would be converted to "ssis-448.mp4"
//
// Args:
//
//	source: the filename to convert
//
// Returns:
//
//	the converted filename
func convertAvFilename(source string) (target string) {
	fileext := strings.ToLower(filepath.Ext(source))

	matched := convertAvFnameRegexp.FindAllStringSubmatch(source, -1)
	if len(matched) == 0 {
		return source
	}

	target = strings.ToLower(matched[0][1])
	target = regexp.MustCompile(`[\-_]hd[\-_]`).ReplaceAllString(target, "-")
	return target + fileext
}
