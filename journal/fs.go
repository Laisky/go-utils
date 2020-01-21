package journal

// fs.go
// create directory and journal id & data files.

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

var (
	// DataFileNameReg journal data file name pattern
	DataFileNameReg = regexp.MustCompile(`^\d{8}_\d{8}\.buf(.gz)?$`)
	// IDFileNameReg journal id file name pattern
	IDFileNameReg   = regexp.MustCompile(`^\d{8}_\d{8}\.ids(.gz)?$`)
	fileGzSuffixReg = regexp.MustCompile(`\.gz$`)

	defaultFileNameTimeLayout       = "20060102"
	defaultFileNameTimeLayoutWithTZ = "20060102-0700"
)

func isFileGZ(fname string) bool {
	return fileGzSuffixReg.MatchString(fname)
}

// PrepareDir `mkdir -p`
func PrepareDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		utils.Logger.Info("create new directory", zap.String("path", path))
		if err = os.MkdirAll(path, DirMode); err != nil {
			return errors.Wrap(err, "try to create buf directory got error")
		}
		return nil
	} else if err != nil {
		return errors.Wrap(err, "try to check buf directory got error")
	}

	if !info.IsDir() {
		return fmt.Errorf("path `%v` should be directory", path)
	}

	return nil
}

// BufFileStat current journal files' stats
type BufFileStat struct {
	NewDataFp, NewIDsFp            *os.File
	OldDataFnames, OldIdsDataFname []string
}

// PrepareNewBufFile create new data & id files, and update BufFileStat
func PrepareNewBufFile(dirPath string, oldFsStat *BufFileStat, isScan, isWithGZ bool, sizeBytes int64) (fsStat *BufFileStat, err error) {
	utils.Logger.Debug("prepare new buf file",
		zap.String("dirpath", dirPath),
		zap.Bool("is_scan", isScan),
	)
	fsStat = &BufFileStat{
		OldDataFnames:   []string{},
		OldIdsDataFname: []string{},
	}

	// scan directories
	var (
		latestDataFName, latestIDsFName string
		fname, absFname                 string
		fs                              []os.FileInfo
	)

	// scan existing buf files.
	// update legacyLoader or first run.
	if isScan || oldFsStat == nil {
		if fs, err = ioutil.ReadDir(dirPath); err != nil {
			return nil, errors.Wrap(err, "try to list dir got error")
		}
		for _, f := range fs {
			fname = f.Name()
			absFname = path.Join(dirPath, fname)

			// macos fs bug, could get removed files
			if _, err := os.Stat(absFname); os.IsNotExist(err) {
				utils.Logger.Warn("file not exists", zap.String("fname", absFname))
				return nil, nil
			}

			if DataFileNameReg.MatchString(fname) {
				utils.Logger.Debug("add data file into queue", zap.String("fname", absFname))
				fsStat.OldDataFnames = append(fsStat.OldDataFnames, absFname)
				if fname > latestDataFName {
					latestDataFName = fname
				}
			} else if IDFileNameReg.MatchString(fname) {
				utils.Logger.Debug("add ids file into queue", zap.String("fname", absFname))
				fsStat.OldIdsDataFname = append(fsStat.OldIdsDataFname, absFname)
				if fname > latestIDsFName {
					latestIDsFName = fname
				}
			}
		}
		utils.Logger.Debug("got old journal files",
			zap.Strings("fs", fsStat.OldDataFnames),
			zap.Strings("fs", fsStat.OldIdsDataFname))
	} else {
		_, latestDataFName = filepath.Split(oldFsStat.NewDataFp.Name())
		_, latestIDsFName = filepath.Split(oldFsStat.NewIDsFp.Name())
	}

	// generate new buf data file name
	// `latestxxxFName` means new buf file name now
	now := utils.Clock.GetUTCNow()
	if latestDataFName == "" {
		latestDataFName = now.Format(defaultFileNameTimeLayout) + "_00000001.buf"
		if isWithGZ {
			latestDataFName += ".gz"
		}
	} else {
		if latestDataFName, err = GenerateNewBufFName(now, latestDataFName, isWithGZ); err != nil {
			return nil, errors.Wrapf(err, "generate new data fname `%v` got error", latestDataFName)
		}
	}

	// generate new buf ids file name
	if latestIDsFName == "" {
		latestIDsFName = now.Format(defaultFileNameTimeLayout) + "_00000001.ids"
		if isWithGZ {
			latestDataFName += ".gz"
		}
	} else {
		if latestIDsFName, err = GenerateNewBufFName(now, latestIDsFName, isWithGZ); err != nil {
			return nil, errors.Wrapf(err, "generate new ids fname `%v` got error", latestIDsFName)
		}
	}

	utils.Logger.Debug("prepare new buf files",
		zap.String("new ids fname", latestIDsFName),
		zap.String("new data fname", latestDataFName))
	if fsStat.NewDataFp, err = OpenBufFile(path.Join(dirPath, latestDataFName), sizeBytes/2); err != nil {
		return nil, err
	}
	if fsStat.NewIDsFp, err = OpenBufFile(path.Join(dirPath, latestIDsFName), 0); err != nil {
		return nil, err
	}

	return fsStat, nil
}

// OpenBufFile create and open file
func OpenBufFile(filepath string, fizeByte int64) (fp *os.File, err error) {
	utils.Logger.Info("create new buf file", zap.String("fname", filepath))
	if fp, err = os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, FileMode); err != nil {
		return nil, errors.Wrapf(err, "open file got error: %+v", filepath)
	}
	if fizeByte != 0 {
		if err = fileutil.Preallocate(fp, fizeByte, false); err != nil {
			return nil, errors.Wrap(err, "try to preallocate fp got error")
		}
	}

	return fp, nil
}

// GenerateNewBufFName return new buf file name depends on current time
// file name looks like `yyyymmddnnnn.ids`, nnnn begin from 0001 for each day
func GenerateNewBufFName(now time.Time, oldFName string, isWithGZ bool) (string, error) {
	utils.Logger.Debug("GenerateNewBufFName", zap.Time("now", now), zap.String("oldFName", oldFName))
	finfo := strings.SplitN(oldFName, ".", 2) // {name, ext}
	if len(finfo) < 2 {
		return oldFName, fmt.Errorf("oldFname `%v` not correct", oldFName)
	}
	fts := finfo[0][:8]
	fidx := finfo[0][9:]
	fext := finfo[1]
	if isWithGZ && !strings.HasSuffix(fext, ".gz") {
		fext += ".gz"
	}

	ft, err := time.Parse(defaultFileNameTimeLayoutWithTZ, fts+"+0000")
	if err != nil {
		return oldFName, errors.Wrapf(err, "parse buf file name `%v` got error", oldFName)
	}

	if now.Sub(ft) > 24*time.Hour {
		return now.Format(defaultFileNameTimeLayout) + "_00000001." + fext, nil
	}

	idx, err := strconv.ParseInt(fidx, 10, 64)
	if err != nil {
		return oldFName, errors.Wrapf(err, "parse buf file's idx `%v` got error", fidx)
	}
	return fmt.Sprintf("%v_%08d.%v", fts, idx+1, fext), nil
}
