package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"

	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
)

var md5DirArg struct {
	SourceDir    string
	TargetDir    string
	RemainSource bool
}

// init registers the "md5dir" command on the root command and binds its persistent flags --input-dir/-i,
// --output-dir/-o and --remain/-r (keep the source after a move) to md5DirArg.
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

		An existing destination is never replaced. Identical content is
		deduplicated (a moved source is removed, a copy is skipped); a destination
		holding different bytes is preserved together with the source, a warning
		is logged and the command exits with an error after processing all files.

			go install github.com/Laisky/go-utils/v6/cmd/gutils@latest

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
		return md5DirFiles(ctx, files, md5DirArg.TargetDir, md5DirArg.RemainSource)
	},
}

// md5DirSaveCaption records the original file name in published files. It is a
// variable so tests can disable the external exiftool side effect.
var md5DirSaveCaption = saveExifCaption

// md5DirFiles places every file into its MD5-addressed destination under
// targetDir, moving it unless remain is true. A destination that already holds
// different bytes is never replaced: both files are preserved, a warning is
// logged and processing continues. It takes the context, the files, the target
// directory and the remain flag, and returns the first hard failure, or an error
// counting preserved conflicts after all files were processed.
func md5DirFiles(ctx context.Context, files []string, targetDir string, remain bool) error {
	var conflicts int
	for i, f := range files {
		if i%100 == 0 {
			glog.Shared.Info("processing", zap.String("ratio", fmt.Sprintf("%d/%d", i, len(files))))
		}

		conflict, err := placeMd5File(ctx, f, targetDir, remain)
		if err != nil {
			return errors.Wrapf(err, "place file %q", f)
		}
		if conflict {
			conflicts++
		}
	}
	if conflicts > 0 {
		return errors.Errorf("%d files were preserved in place because their destination "+
			"holds different content or could not be verified", conflicts)
	}

	return nil
}

// placeMd5File publishes f at targetDir/<md5[:2]>/<md5><lowercase ext> without
// ever replacing an existing destination. Moves publish with a no-replace hard
// link and then remove the source, falling back to an exclusive copy when hard
// links are unavailable; copies always use exclusive creation. When the
// destination exists, its bytes are compared with f: identical content is
// deduplicated (move removes the redundant source, copy skips), while different
// content or a comparison failure preserves both files and is reported as a
// conflict. It takes the context, the source path, the target directory and the
// remain flag, and returns whether a conflict was preserved or a hard error.
func placeMd5File(ctx context.Context, f, targetDir string, remain bool) (conflict bool, err error) {
	// Hash under ctx so cancellation stops before any destination is touched.
	// A zero byte cap selects gutils.DefaultFileHashMaxBytes, as FileHash does.
	hashedBytes, err := gutils.FileHashWithContext(ctx, gutils.HashTypeMD5, f, 0)
	if err != nil {
		return false, errors.Wrapf(err, "calculate hash for file %q", f)
	}
	hashed := hex.EncodeToString(hashedBytes)

	outputDir := filepath.Join(targetDir, hashed[:2])
	if err = os.MkdirAll(outputDir, 0755); err != nil {
		return false, errors.Wrapf(err, "mkdir %q", outputDir)
	}

	target := filepath.Join(outputDir, hashed+strings.ToLower(filepath.Ext(f)))
	if filepath.Clean(f) == filepath.Clean(target) {
		glog.Shared.Debug("file already stored at its content address", zap.String("file", f))
		return false, nil
	}

	err = publishMd5File(f, target, remain)
	if err == nil {
		// save raw file name into file's EXIF
		if err = md5DirSaveCaption(ctx, target, filepath.Base(f)); err != nil {
			glog.Shared.Warn("save caption into exif", zap.Error(err))
		}
		glog.Shared.Info("moved file", zap.String("from", f), zap.String("to", target))
		return false, nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return false, err
	}

	return resolveMd5Conflict(f, target, remain), nil
}

// publishMd5File places f at target without replacing any existing entry. It takes
// the source, the destination and the remain flag, and returns an error matching
// fs.ErrExist when the destination is occupied, or another failure.
func publishMd5File(f, target string, remain bool) error {
	if remain {
		return errors.Wrapf(gutils.CopyFile(f, target, gutils.WithFileMode(0644)),
			"copy file from %q to %q", f, target)
	}

	linkErr := os.Link(f, target)
	if linkErr != nil && !errors.Is(linkErr, fs.ErrExist) {
		glog.Shared.Debug("hard link unavailable, falling back to exclusive copy",
			zap.String("from", f), zap.String("to", target), zap.Error(linkErr))
		if err := gutils.CopyFile(f, target, gutils.WithFileMode(0644)); err != nil {
			return errors.Wrapf(err, "copy file from %q to %q", f, target)
		}
	} else if linkErr != nil {
		return errors.Wrapf(linkErr, "link file from %q to %q", f, target)
	}

	if err := os.Remove(f); err != nil {
		return errors.Wrapf(err, "remove moved source %q", f)
	}
	return nil
}

// resolveMd5Conflict handles an occupied destination without replacing it.
// Identical bytes are deduplicated; different bytes, a shared inode or a failed
// comparison preserve both files and log a warning. It takes the source, the
// destination and the remain flag, and returns true when a conflict was preserved.
func resolveMd5Conflict(f, target string, remain bool) bool {
	cmp, err := compareFileContent(target, f)
	if err != nil {
		glog.Shared.Warn("cannot verify existing content-addressed destination; preserving both files",
			zap.String("file", f), zap.String("destination", target), zap.Error(err))
		return true
	}
	if os.SameFile(cmp.first, cmp.second) {
		glog.Shared.Info("destination is already a link to this file; leaving both names",
			zap.String("file", f), zap.String("destination", target))
		return false
	}
	if !cmp.equal {
		glog.Shared.Warn("different content already occupies the content-addressed destination; "+
			"preserving both files", zap.String("file", f), zap.String("destination", target))
		return true
	}
	if remain {
		glog.Shared.Info("identical content already stored; skip copy",
			zap.String("file", f), zap.String("destination", target))
		return false
	}
	if err := removeIfUnchanged(f, cmp.second); err != nil {
		glog.Shared.Warn("identical content already stored but source could not be removed safely",
			zap.String("file", f), zap.String("destination", target), zap.Error(err))
		return true
	}
	glog.Shared.Info("identical content already stored; removed redundant source",
		zap.String("file", f), zap.String("destination", target))
	return false
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

// checkMd5DirArg validates and normalizes the md5dir arguments stored in md5DirArg. It requires a non-empty
// source directory, defaults the target directory to the source directory when it is empty, and rewrites both
// to absolute paths in place. It returns an error when the source directory is missing or an absolute path
// cannot be resolved, and nil otherwise.
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
