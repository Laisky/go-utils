package cmd

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/rivo/duplo"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	gutils "github.com/Laisky/go-utils/v4"
	glog "github.com/Laisky/go-utils/v4/log"
)

var removeDupArg struct {
	Dir string
	Dry bool
}

type dupFile struct {
	path      string
	hash      string
	sizeBytes int64
}

func init() {
	rootCmd.AddCommand(removeDupCMD)
	removeDupCMD.PersistentFlags().StringVarP(&removeDupArg.Dir,
		"dir", "d", "", "directory")
	removeDupCMD.PersistentFlags().BoolVar(&removeDupArg.Dry,
		"dry", false, "dry run")
}

// removeDupCMD encrypt files
var removeDupCMD = &cobra.Command{
	Use:   "remove-dup",
	Short: "remove duplicate files",
	Long: gutils.Dedent(`
		Find and remove duplicate files or images

			go install github.com/Laisky/go-utils/v4/cmd/gutils@latest

			gutils remove-dup -d examples/images --dry
	`),
	Args: NoExtraArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := removeDuplicate(removeDupArg.Dry, removeDupArg.Dir); err != nil {
			glog.Shared.Panic("remove duplicate", zap.Error(err))
		}
	},
}

func removeDuplicate(dry bool, dir string) error {
	files, err := gutils.ListFilesInDir(dir, gutils.ListFilesInDirRecursive())
	if err != nil {
		return errors.Wrapf(err, "list files in %q", dir)
	}

	glog.Shared.Debug("list files", zap.Int("n", len(files)))
	similarStore := duplo.New()
	fileHashes := &sync.Map{} // map[string]*dupFile{}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var pool errgroup.Group
	pool.SetLimit(runtime.NumCPU())

	for i, fpath := range files {
		select {
		case <-ticker.C:
			glog.Shared.Debug("scanning...",
				zap.String("ratio", fmt.Sprintf("%d/%d", i, len(files))))
		default:
		}

		fpath := fpath
		pool.Go(func() (err error) {
			glog.Shared.Debug("check duplicate by content hash", zap.String("file", fpath))
			if deleted, err := checkDupByHash(dry, fileHashes, fpath); err != nil {
				glog.Shared.Warn("checkDupByHash", zap.String("file", fpath), zap.Error(err))
			} else if deleted {
				return nil
			}

			glog.Shared.Debug("check duplicate by similar images", zap.String("file", fpath))
			// maybe some day, there will add some other checker
			//nolint: staticcheck // SA4006: this value of `deleted` is never used
			if _, err := checkDupByImageSimilar(dry, similarStore, fpath); err != nil {
				glog.Shared.Warn("checkDupByImageSimilar", zap.String("file", fpath), zap.Error(err))
			}

			return nil
		})
	}

	if err := pool.Wait(); err != nil {
		return errors.Wrap(err, "wait for pool")
	}

	return nil
}

func checkDupByImageSimilar(dry bool, store *duplo.Store, fpath string) (deleted bool, err error) {
	fp, err := os.Open(fpath)
	if err != nil {
		return false, errors.Wrapf(err, "open file %q", fpath)
	}
	defer gutils.SilentClose(fp)

	var img image.Image
	ext := strings.ToLower(filepath.Ext(fpath))
	switch ext {
	case ".jpeg", ".jpg", ".jfif":
		// ext = ".jpg" //nolint: ineffassign
		if img, err = jpeg.Decode(fp); err != nil {
			return false, errors.Wrapf(err, "decode jpeg file %q", fpath)
		}
	case ".png":
		if img, err = png.Decode(fp); err != nil {
			return false, errors.Wrapf(err, "decode png file %q", fpath)
		}
	case ".gif":
		if img, err = gif.Decode(fp); err != nil {
			return false, errors.Wrapf(err, "decode gif file %q", fpath)
		}
	default:
		glog.Shared.Warn("skip for unsupported image", zap.String("file", fpath))
		return false, nil
	}

	glog.Shared.Debug("check similar for images", zap.String("file", fpath))
	hash, _ := duplo.CreateHash(img)
	matched := store.Query(hash)
	if len(matched) == 0 {
		return false, nil
	}

	sort.Sort(matched)
	otherFile := matched[0]
	if otherFile.Score > -60 { // experience threshold
		return false, nil
	}

	deleted = true
	otherFp, ok := otherFile.ID.(string)
	if !ok {
		return false, errors.Errorf("invalid file id type %T", otherFile.ID)
	}
	keepCurrentFile, err := fileSizeBiggerThan(fpath, otherFp)
	if err != nil {
		return false, errors.Wrap(err, "compare file size")
	}

	deletePath := fpath
	keepPath := otherFp
	if keepCurrentFile {
		store.Delete(otherFile)
		store.Add(fpath, hash)
		deletePath = otherFp
		keepPath = fpath
	}

	glog.Shared.Info("remove similar image",
		zap.Float64("score", otherFile.Score),
		zap.String("keep", keepPath),
		zap.String("remove", deletePath))
	if !dry {
		return deleted, removeFile(deletePath)
	}

	if !deleted {
		store.Add(fpath, hash)
	}

	return deleted, nil
}

func fileSizeBiggerThan(fp1, fp2 string) (bool, error) {
	finfo1, err := os.Stat(fp1)
	if err != nil {
		return false, errors.Wrapf(err, "get stat for file %q", fp1)
	}

	finfo2, err := os.Stat(fp2)
	if err != nil {
		return false, errors.Wrapf(err, "get stat for file %q", fp2)
	}

	return finfo1.Size() > finfo2.Size(), nil
}

func checkDupByHash(dry bool, hashes *sync.Map, fpath string) (deleted bool, err error) {
	fhashBytes, err := gutils.FileHash(gutils.HashTypeSha1, fpath)
	if err != nil {
		return false, errors.Wrapf(err, "get hash of file %q", fpath)
	}
	fhash := hex.EncodeToString(fhashBytes)

	fstat, err := os.Stat(fpath)
	if err != nil {
		return false, errors.Wrapf(err, "get stat of file %q", fpath)
	}

	cacheItem := &dupFile{
		path:      fpath,
		hash:      fhash,
		sizeBytes: fstat.Size(),
	}

	if vi, loaded := hashes.LoadOrStore(fhash, cacheItem); loaded {
		raw := vi.(*dupFile) //nolint:forcetypeassert
		glog.Shared.Info("remove duplicate since same hash",
			zap.String("remove", fpath),
			zap.String("keep", raw.path),
		)
		if !dry {
			return true, removeFile(fpath)
		}

		return true, nil
	}

	return false, nil
}

func removeFile(fpath string) error {
	if err := os.Remove(fpath); err != nil {
		return errors.Wrapf(err, "remove file %q", fpath)
	}

	return nil
}
