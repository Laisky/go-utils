package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v4"
	glog "github.com/Laisky/go-utils/v4/log"
)

var md5DirArg struct {
	SourceDir    string
	TargetDir    string
	RemainSource bool
}

func init() {
	rootCmd.AddCommand(md5DirCMD)
	md5DirCMD.PersistentFlags().StringVarP(&md5DirArg.SourceDir,
		"input-dir", "i", "", "source directory")
	md5DirCMD.PersistentFlags().StringVarP(&md5DirArg.TargetDir,
		"output-dir", "o", "", "target directory")
	md5DirCMD.PersistentFlags().BoolVarP(&md5DirArg.RemainSource,
		"remain", "r", false, "do not delete source after move")
}

// md5DirCMD encrypt files
var md5DirCMD = &cobra.Command{
	Use:   "md5dir",
	Short: "move files to md5 hierarchy directories",
	Long: gutils.Dedent(`
		Move files to hierarchy directories splitted by prefix of md5

			go install github.com/Laisky/go-utils/v4/cmd/gutils@latest

			gutils md5dir -i examples/md5dir/
	`),
	Args: NoExtraArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()
		if err := checkMd5DirArg(); err != nil {
			return errors.Wrap(err, "command args invalid")
		}

		files, err := gutils.ListFilesInDir(md5DirArg.SourceDir)
		if err != nil {
			return errors.Wrap(err, "list files in source dir")
		}

		glog.Shared.Info("try to move files",
			zap.Int("files", len(files)),
			zap.String("from", md5DirArg.SourceDir),
			zap.String("to", md5DirArg.TargetDir))
		for i, f := range files {
			if i%100 == 0 {
				glog.Shared.Info("processing", zap.String("ratio", fmt.Sprintf("%d/%d", i, len(files))))
			}

			hashedBytes, err := gutils.FileHash(gutils.HashTypeMD5, f)
			if err != nil {
				return errors.Wrapf(err, "calculate hash for file %q", f)
			}
			hashed := hex.EncodeToString(hashedBytes)

			outputDir := filepath.Join(md5DirArg.TargetDir, hashed[:2])
			if err = os.MkdirAll(outputDir, 0755); err != nil {
				return errors.Wrapf(err, "mkdir %q", outputDir)
			}

			target := filepath.Join(outputDir, hashed+strings.ToLower(filepath.Ext(f)))
			if !md5DirArg.RemainSource { // move file
				if err = os.Rename(f, target); err != nil {
					return errors.Wrapf(err, "move file from %q to %q", f, target)
				}
			} else {
				if err = gutils.CopyFile(f, target,
					gutils.WithFileFlag(os.O_CREATE|os.O_WRONLY),
					gutils.Overwrite(),
					gutils.WithFileMode(0644),
				); err != nil {
					return errors.Wrapf(err, "copy file from %q to %q", f, target)
				}
			}

			// save raw file name into file's EXIF
			if err = saveExifCaption(ctx, target, filepath.Base(f)); err != nil {
				glog.Shared.Warn("save caption into exif", zap.Error(err))
			}

			glog.Shared.Info("moved file", zap.String("from", f), zap.String("to", target))
		}

		return nil
	},
}

// saveExifCaption save caption into exif
func saveExifCaption(ctx context.Context, fpath string, caption string) error {
	// check whether exiftool exists
	exePath, err := exec.LookPath("exiftool")
	if err != nil {
		return errors.Wrap(err, "exiftool not found")
	}

	// sanitize caption
	caption = strings.ReplaceAll(caption, `"`, `\"`)
	_, err = gutils.RunCMD(ctx, exePath, "-caption="+caption, fpath)
	if err != nil {
		return errors.Wrap(err, "run exiftool to save caption")
	}

	return nil
}

func checkMd5DirArg() (err error) {
	if md5DirArg.SourceDir == "" {
		return errors.Errorf("--intput-dir should not be empty")
	}
	if md5DirArg.SourceDir, err = filepath.Abs(md5DirArg.SourceDir); err != nil {
		return errors.Wrap(err, "get abs source dir")
	}

	if md5DirArg.TargetDir == "" {
		if md5DirArg.SourceDir != "" {
			md5DirArg.TargetDir = md5DirArg.SourceDir
		} else {
			return errors.Errorf("--output-dir should not be empty")
		}
	}
	if md5DirArg.TargetDir, err = filepath.Abs(md5DirArg.TargetDir); err != nil {
		return errors.Wrap(err, "get abs target dir")
	}

	return nil
}
