package journal

import (
	"io"
	"os"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

type LegacyLoader struct {
	dataFNames, idsFNames []string
	ctx                   *legacyCtx
}

type legacyCtx struct {
	isNeedReload                bool // prepare datafp for `Load`
	isReadyReload               bool // alreddy update `dataFNames`
	ids                         *Int64Set
	dataFileIdx, dataFileMaxIdx int
	dataFp                      *os.File
	decoder                     *DataDecoder
}

func NewLegacyLoader(dataFNames, idsFNames []string) *LegacyLoader {
	utils.Logger.Info("new legacy loader", zap.Strings("dataFiles", dataFNames), zap.Strings("idsFiles", idsFNames))
	l := &LegacyLoader{
		dataFNames: dataFNames,
		idsFNames:  idsFNames,
		ctx: &legacyCtx{
			ids:           NewInt64Set(),
			isNeedReload:  true,
			isReadyReload: true && len(dataFNames) != 0,
		},
	}
	return l
}

func (l *LegacyLoader) AddID(id int64) {
	l.ctx.ids.Add(id)
}

func (l *LegacyLoader) Reset(dataFNames, idsFNames []string) {
	utils.Logger.Info("reset legacy loader", zap.Strings("dataFiles", dataFNames), zap.Strings("idsFiles", idsFNames))
	l.dataFNames = dataFNames
	l.idsFNames = idsFNames
	l.ctx.isReadyReload = true
}

// removeFile delete file, should run sync to avoid dirty files
func (l *LegacyLoader) removeFile(fpath string) {
	if err := os.Remove(fpath); err != nil {
		utils.Logger.Error("try to delete file got error",
			zap.String("file", fpath),
			zap.Error(err))
	}
	utils.Logger.Info("remove buf file", zap.String("file", fpath))
}

func (l *LegacyLoader) Load(data *Data) (err error) {
	if l.ctx.isNeedReload { // refresh ctx
		if !l.ctx.isReadyReload { // legacy files not prepared
			return io.EOF
		}

		utils.Logger.Debug("reload ids & data file idx")
		err = l.LoadAllids(l.ctx.ids)
		if err != nil {
			// use origin ids if got error in LoadAllids
			utils.Logger.Error("try to load all ids got error", zap.Error(err))
		}

		l.ctx.dataFileMaxIdx = len(l.dataFNames) - 1
		l.ctx.dataFileIdx = 0
		l.ctx.isNeedReload = false
	}

READ_NEW_FILE:
	if l.ctx.dataFp == nil {
		utils.Logger.Debug("read new data file", zap.String("fname", l.dataFNames[l.ctx.dataFileIdx]))
		l.ctx.dataFp, err = os.Open(l.dataFNames[l.ctx.dataFileIdx])
		if err != nil {
			return errors.Wrap(err, "try to open data file got error")
		}
		l.ctx.decoder = NewDataDecoder(l.ctx.dataFp)
	}

READ_NEW_LINE:
	if err = l.ctx.decoder.Read(data); err != nil {
		if err == io.EOF {
			if l.ctx.dataFileIdx == l.ctx.dataFileMaxIdx { // all data files finished
				utils.Logger.Debug("all data files finished")
				return io.EOF
			}
		} else { // current file is broken
			utils.Logger.Error("try to load data file got error", zap.Error(err))
		}

		// read new file
		if err = l.ctx.dataFp.Close(); err != nil {
			utils.Logger.Error("try to close file got error", zap.String("file", l.dataFNames[l.ctx.dataFileIdx]), zap.Error(err))
		}

		l.ctx.dataFp = nil
		l.ctx.dataFileIdx++
		utils.Logger.Debug("read new data file", zap.String("fname", l.dataFNames[l.ctx.dataFileIdx]))
		goto READ_NEW_FILE
	}

	if l.ctx.ids.CheckAndRemove(data.ID) { // duplicated
		// utils.Logger.Debug("data already consumed", zap.Int64("id", id))
		goto READ_NEW_LINE
	}

	// utils.Logger.Debug("load unconsumed data", zap.Int64("id", id))
	return nil
}

func (l *LegacyLoader) LoadMaxId() (maxId int64, err error) {
	utils.Logger.Debug("LoadMaxId...")
	var (
		fp *os.File
		id int64
	)
	startTs := time.Now()
	for _, fname := range l.idsFNames {
		// utils.Logger.Debug("load ids from file", zap.String("fname", fname))
		fp, err = os.Open(fname)
		if err != nil {
			return 0, errors.Wrapf(err, "try to open file `%v` to load maxid got error", fname)
		}
		defer fp.Close()

		idsDecoder := NewIdsDecoder(fp)
		id, err = idsDecoder.LoadMaxId()
		if err != nil {
			return 0, errors.Wrapf(err, "try to read file `%v` got error", fname)
		}
		if id < maxId {
			maxId = id
		}
	}

	utils.Logger.Info("load max id done", zap.Float64("sec", time.Now().Sub(startTs).Seconds()))
	return id, nil
}

func (l *LegacyLoader) LoadAllids(ids *Int64Set) (allErr error) {
	utils.Logger.Debug("LoadAllids...")
	var (
		err error
		fp  *os.File
	)

	startTs := time.Now()
	for _, fname := range l.idsFNames {
		// utils.Logger.Debug("load ids from file", zap.String("fname", fname))
		fp, err = os.Open(fname)
		defer fp.Close()
		if err != nil {
			allErr = errors.Wrapf(err, "try to open ids file `%v` to load all ids got error", fname)
			utils.Logger.Error("try to open ids file to load all ids got error",
				zap.String("fname", fname),
				zap.Error(err))
		}

		idsDecoder := NewIdsDecoder(fp)
		if err = idsDecoder.ReadAllToInt64Set(ids); err != nil {
			allErr = errors.Wrapf(err, "try to read ids file `%v` got error", fname)
			utils.Logger.Error("try to read ids file got error",
				zap.String("fname", fname),
				zap.Error(err))
		}
	}

	utils.Logger.Info("load all ids done", zap.Float64("sec", time.Now().Sub(startTs).Seconds()))
	return allErr
}

func (l *LegacyLoader) Clean() error {
	if l.dataFNames != nil {
		for _, f := range l.dataFNames {
			l.removeFile(f)
		}
		l.dataFNames = nil
	}

	if l.idsFNames != nil {
		for _, f := range l.idsFNames {
			l.removeFile(f)
		}
		l.idsFNames = nil
	}

	l.ctx.dataFp.Close()
	l.ctx.isNeedReload = true
	l.ctx.isReadyReload = false
	l.ctx.dataFp = nil
	utils.Logger.Info("clean all legacy")
	return nil
}
