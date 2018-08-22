package journal

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var (
	DataFileNameReg = regexp.MustCompile(`\d{8}\d{4}\.buf`)
	IdFileNameReg   = regexp.MustCompile(`\d{8}\d{4}\.ids`)
	layout          = "20060102"
	layoutWithTZ    = "20060102-0700"
)

func PrepareDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		utils.Logger.Info("create new directory", zap.String("path", path))
		err = os.MkdirAll(path, 0774)
		if err != nil {
			return errors.Wrap(err, "try to create buf directory got error")
		}
	} else if err != nil {
		return errors.Wrap(err, "try to check buf directory got error")
	}

	if !info.IsDir() {
		return fmt.Errorf("path `%v` should be directory", path)
	}

	return nil
}

type BufFileStat struct {
	NewDataFName, NewIdsDataFname  string
	OldDataFnames, OldIdsDataFname []string
}

func PrepareNewBufFile(dirPath string) (ret *BufFileStat, err error) {
	utils.Logger.Debug("PrepareNewBufFile", zap.String("dirPath", dirPath))
	ret = &BufFileStat{
		OldDataFnames:   []string{},
		OldIdsDataFname: []string{},
	}

	// scan directories
	var latestDataFName, latestIDFName, fname string
	err = filepath.Walk(dirPath, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fname = info.Name()

		if DataFileNameReg.MatchString(fname) {
			ret.OldDataFnames = append(ret.OldDataFnames, path.Join(dirPath, fname))
			if fname > latestDataFName {
				latestDataFName = fname
			}
		} else if IdFileNameReg.MatchString(fname) {
			ret.OldIdsDataFname = append(ret.OldIdsDataFname, path.Join(dirPath, fname))
			if fname > latestIDFName {
				latestIDFName = fname
			}
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "walk dirpath `%v` got error", dirPath)
	}

	// generate new buf file name
	now := utils.UTCNow()
	if latestDataFName == "" {
		latestDataFName = now.Format(layout) + "0001.buf"
	} else {
		latestDataFName, err = GenerateNewBufFName(now, latestDataFName)
		if err != nil {
			return nil, errors.Wrapf(err, "generate new data fname `%v` got error", latestDataFName)
		}
	}

	if latestIDFName == "" {
		latestIDFName = now.Format(layout) + "0001.ids"
	} else {
		latestIDFName, err = GenerateNewBufFName(now, latestIDFName)
		if err != nil {
			return nil, errors.Wrapf(err, "generate new data fname `%v` got error", latestDataFName)
		}
	}

	utils.Logger.Debug("PrepareNewBufFile", zap.String("new ids fname", latestIDFName), zap.String("new data fname", latestDataFName))
	ret.NewDataFName = path.Join(dirPath, latestDataFName)
	ret.NewIdsDataFname = path.Join(dirPath, latestIDFName)
	return ret, nil
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
	fidx := finfo[0][8:]
	fext := finfo[1]

	ft, err := time.Parse(layoutWithTZ, fts+"+0000")
	if err != nil {
		return oldFName, errors.Wrapf(err, "parse buf file name `%v` got error", oldFName)
	}

	if now.Sub(ft) > 24*time.Hour {
		return now.Format(layout) + "0001." + fext, nil
	}

	idx, err := strconv.ParseInt(fidx, 10, 64)
	if err != nil {
		return oldFName, errors.Wrapf(err, "parse buf file's idx `%v` got error", fidx)
	}
	return fmt.Sprintf("%v%04d.%v", fts, idx+1, fext), nil
}
