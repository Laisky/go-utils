package journal

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var (
	DataFileNameReg = regexp.MustCompile(`\d{8}_\d{8}\.buf`)
	IdFileNameReg   = regexp.MustCompile(`\d{8}_\d{8}\.ids`)
	layout          = "20060102"
	layoutWithTZ    = "20060102-0700"
)

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

type BufFileStat struct {
	NewDataFp, NewIdsDataFp, NextDataFp, NextIdsDataFp *os.File
	OldDataFnames, OldIdsDataFname                     []string
}

func PrepareNewBufFile(dirPath string, isScan bool) (ret *BufFileStat, err error) {
	utils.Logger.Debug("PrepareNewBufFile", zap.String("dirPath", dirPath))
	ret = &BufFileStat{
		OldDataFnames:   []string{},
		OldIdsDataFname: []string{},
	}

	// scan directories
	var (
		latestDataFName, latestIDsFName string
		fname, absFname                 string
		fs                              []os.FileInfo
	)

	// scan existing buf files
	if isScan {
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
				ret.OldDataFnames = append(ret.OldDataFnames, absFname)
				if fname > latestDataFName {
					latestDataFName = fname
				}
			} else if IdFileNameReg.MatchString(fname) {
				utils.Logger.Debug("add ids file into queue", zap.String("fname", absFname))
				ret.OldIdsDataFname = append(ret.OldIdsDataFname, absFname)
				if fname > latestIDsFName {
					latestIDsFName = fname
				}
			}
		}
		utils.Logger.Debug("got data files", zap.Strings("fs", ret.OldDataFnames))
		utils.Logger.Debug("got ids files", zap.Strings("fs", ret.OldIdsDataFname))
	}

	// generate new buf data file name
	// `latestxxxFName` means new buf file name now
	now := utils.UTCNow()
	if latestDataFName == "" {
		latestDataFName = now.Format(layout) + "_00000001.buf"
	} else {
		if latestDataFName, err = GenerateNewBufFName(now, latestDataFName); err != nil {
			return nil, errors.Wrapf(err, "generate new data fname `%v` got error", latestDataFName)
		}
	}

	// generate new buf ids file name
	if latestIDsFName == "" {
		latestIDsFName = now.Format(layout) + "_00000001.ids"
	} else {
		if latestIDsFName, err = GenerateNewBufFName(now, latestIDsFName); err != nil {
			return nil, errors.Wrapf(err, "generate new ids fname `%v` got error", latestIDsFName)
		}
	}

	utils.Logger.Debug("PrepareNewBufFile",
		zap.String("new ids fname", latestIDsFName),
		zap.String("new data fname", latestDataFName))
	if ret.NewDataFp, err = OpenBufFile(path.Join(dirPath, latestDataFName)); err != nil {
		return nil, err
	}
	if ret.NewIdsDataFp, err = OpenBufFile(path.Join(dirPath, latestIDsFName)); err != nil {
		return nil, err
	}

	return ret, nil
}

func OpenBufFile(filepath string) (fp *os.File, err error) {
	utils.Logger.Info("create new buf file", zap.String("fname", filepath))
	if fp, err = os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, FileMode); err != nil {
		return nil, errors.Wrapf(err, "open file got error: %+v", filepath)
	}

	return fp, nil
}

// GenerateNewBufFName return new buf file name depends on current time
// file name looks like `yyyymmddnnnn.ids`, nnnn begin from 0001 for each day
func GenerateNewBufFName(now time.Time, oldFName string) (string, error) {
	utils.Logger.Debug("GenerateNewBufFName", zap.Time("now", now), zap.String("oldFName", oldFName))
	finfo := strings.Split(oldFName, ".") // {name, ext}
	if len(finfo) < 2 {
		return oldFName, fmt.Errorf("oldFname `%v` not correct", oldFName)
	}
	fts := finfo[0][:8]
	fidx := finfo[0][9:]
	fext := finfo[1]

	ft, err := time.Parse(layoutWithTZ, fts+"+0000")
	if err != nil {
		return oldFName, errors.Wrapf(err, "parse buf file name `%v` got error", oldFName)
	}

	if now.Sub(ft) > 24*time.Hour {
		return now.Format(layout) + "_00000001." + fext, nil
	}

	idx, err := strconv.ParseInt(fidx, 10, 64)
	if err != nil {
		return oldFName, errors.Wrapf(err, "parse buf file's idx `%v` got error", fidx)
	}
	return fmt.Sprintf("%v_%08d.%v", fts, idx+1, fext), nil
}
