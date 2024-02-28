package cmd

import (
	"fmt"
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
	renameCMD.AddCommand(renameAvCmd)
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
	RunE: func(_ *cobra.Command, _ []string) error {
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

var (
	convertAvFnameRegexp = regexp.MustCompile(`(?P<name>(?:fc2-)?[a-zA-Z]+(?:\-hd)?\-\d+(?:\-\d+)?)`)
	// convertAvCaribRegexp match carib av filename
	//
	// example: 080422-003-CARIB
	convertAvCaribRegexp = regexp.MustCompile(`(\d+)\-(\d+)\-(carib|caribbean|1pon)`)
	// convertAvFnameWithBracket match av filename with bracket
	//
	// examples:
	//  * (Caribbean)(102018-777)これが私の望むセックス 碧しの.jpg
	//  * (FC2)(843225)女子アナ候補生パイパン美女 種付けまんこに連続生ハメ精子押し込み受精確実→潮吹きながら痙攣アクメ悶絶イキ.mp4
	//  * (Heyzo)(1723)弄ばれる童顔女教師 千野くるみ.jpg
	//  * (Tokyo-Hot)(n1307)東熱激情 欲情綺麗なお姉さん特集 part2.jpg
	convertAvFnameWithBracket = regexp.MustCompile(`\(([\w-]+)\)\((n?[\d-]+)\)`)
)

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
	dir := filepath.Dir(source)
	lowerSrc := strings.ToLower(source)
	fileext := filepath.Ext(lowerSrc)

	lowerSrc = strings.ReplaceAll(lowerSrc, "_", "-")
	lowerSrc = strings.ReplaceAll(lowerSrc, " ", "-")

	// extract numbers in bracket
	matched := convertAvFnameWithBracket.FindAllStringSubmatch(lowerSrc, -1)
	if len(matched) > 0 && len(matched[0]) > 2 {
		target = matched[0][1] + "-" + matched[0][2]
	}

	// extract carib/1pon that digits are ahead of characters
	if target == "" {
		matched := convertAvCaribRegexp.FindAllStringSubmatch(lowerSrc, -1)
		if len(matched) > 0 && len(matched[0]) > 2 {
			target = fmt.Sprintf("%s-%s-%s", matched[0][3], matched[0][1], matched[0][2])
		}
	}

	if target == "" {
		matched := convertAvFnameRegexp.FindAllStringSubmatch(lowerSrc, -1)
		if len(matched) == 0 {
			return source
		}

		target = matched[0][1]
		target = regexp.MustCompile(`\-hd\-`).ReplaceAllString(target, "-")
	}

	if !strings.Contains(target, "caribbean") {
		target = strings.ReplaceAll(target, "carib", "caribbean")
	}
	target = strings.ReplaceAll(target, "caribbeancom", "caribbean")
	target = strings.ReplaceAll(target, "1pon", "1pondo")
	return filepath.Join(dir, target+fileext)
}
