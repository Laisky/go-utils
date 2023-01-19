package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v2"
	glog "github.com/Laisky/go-utils/v2/log"
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

			go install github.com/Laisky/go-utils/v2/cmd/gutils@latest

			gutils md5dir -i examples/md5dir/
	`),
	Args: NoExtraArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := checkMd5DirArg(); err != nil {
			glog.Shared.Panic("command args invalid", zap.Error(err))
		}

		files, err := gutils.ListFilesInDir(md5DirArg.SourceDir)
		if err != nil {
			glog.Shared.Panic("list files in source dir", zap.Error(err))
		}

		glog.Shared.Info("try to move files",
			zap.Int("files", len(files)),
			zap.String("from", md5DirArg.SourceDir),
			zap.String("to", md5DirArg.TargetDir))
		for i, f := range files {
			if i%100 == 0 {
				glog.Shared.Info("processing", zap.String("ratio", fmt.Sprintf("%d/%d", i, len(files))))
			}

			hashed, err := gutils.FileMD5(f)
			if err != nil {
				glog.Shared.Panic("calculate hash for file",
					zap.Error(err), zap.String("file", f))
			}

			outputDir := filepath.Join(md5DirArg.TargetDir, hashed[:2])
			if err = os.MkdirAll(outputDir, 0755); err != nil {
				glog.Shared.Panic("mkdir", zap.String("dir", outputDir), zap.Error(err))
			}

			target := filepath.Join(outputDir, hashed+strings.ToLower(filepath.Ext(f)))
			if err = gutils.CopyFile(f, target,
				gutils.WithFileFlag(os.O_CREATE|os.O_WRONLY),
				gutils.WithFileMode(0644),
			); err != nil {
				glog.Shared.Panic("copy file", zap.Error(err),
					zap.String("from", f), zap.String("to", target),
				)
			}

			glog.Shared.Info("moved file", zap.String("from", f), zap.String("to", target))
			if !md5DirArg.RemainSource {
				if err = os.Remove(f); err != nil {
					glog.Shared.Panic("remove file", zap.Error(err), zap.String("file", f))
				}
			}
		}
	},
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
