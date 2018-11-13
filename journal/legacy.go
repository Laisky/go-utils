package journal

import (
	"io"
	"os"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/RoaringBitmap/roaring"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type LegacyLoader struct {
	dataFNames, idsFNames []string
	ctx                   *legacyCtx
}

type legacyCtx struct {
	ids                         *roaring.Bitmap
	dataFileIdx, dataFileMaxIdx int
	dataFp                      *os.File
	decoder                     *DataDecoder
}

func NewLegacyLoader(dataFNames, idsFNames []string) *LegacyLoader {
	utils.Logger.Info("NewLegacyLoader...", zap.Strings("dataFiles", dataFNames), zap.Strings("idsFiles", idsFNames))
	return &LegacyLoader{
		dataFNames: dataFNames,
		idsFNames:  idsFNames,
		ctx:        &legacyCtx{},
	}
}

func (l *LegacyLoader) Load(data *map[string]interface{}) (err error) {
	utils.Logger.Debug("LegacyLoader.Load...")
	if l.ctx.ids == nil { // first run
		if len(l.dataFNames) == 0 { // no legacy files
			return io.EOF
		}

		l.ctx.ids, err = l.LoadAllids()
		if err != nil {
			return errors.Wrap(err, "try to load all ids got error")
		}

		l.ctx.dataFileMaxIdx = len(l.dataFNames) - 1
		l.ctx.dataFileIdx = 0
	}
	var id int64

READ_NEW_FILE:
	if l.ctx.dataFp == nil {
		l.ctx.dataFp, err = os.Open(l.dataFNames[l.ctx.dataFileIdx])
		if err != nil {
			return errors.Wrap(err, "try to open data file got error")
		}
		l.ctx.decoder = NewDataDecoder(l.ctx.dataFp)
	}

READ_NEW_LINE:
	if err = l.ctx.decoder.Read(data); err == io.EOF {
		if l.ctx.dataFileIdx == l.ctx.dataFileMaxIdx { // all data files finished
			utils.Logger.Debug("all data files finished")
			return io.EOF
		}

		if err = l.ctx.dataFp.Close(); err != nil {
			utils.Logger.Error("try to close file got error", zap.String("file", l.dataFNames[l.ctx.dataFileIdx]), zap.Error(err))
		}
		l.ctx.dataFp = nil
		l.ctx.dataFileIdx++
		utils.Logger.Debug("read new data file", zap.String("fname", l.dataFNames[l.ctx.dataFileIdx]))
		goto READ_NEW_FILE
	} else if err != nil {
		return errors.Wrap(err, "try to load data file got error")
	}

	id = GetId(*data)
	if l.ctx.ids.ContainsInt(int(id)) { // duplicated
		utils.Logger.Debug("data already consumed", zap.Int64("id", id))
		goto READ_NEW_LINE
	}

	utils.Logger.Debug("load unconsumed data", zap.Int64("id", id))
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
		utils.Logger.Debug("load ids from file", zap.String("fname", fname))
		fp, err = os.Open(fname)
		defer fp.Close()
		if err != nil {
			return 0, errors.Wrapf(err, "try to open file `%v` got error", fname)
		}

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

func (l *LegacyLoader) LoadAllids() (ids *roaring.Bitmap, err error) {
	utils.Logger.Debug("LoadAllids...")
	var (
		fp     *os.File
		newIds *roaring.Bitmap
	)
	ids = roaring.New()
	startTs := time.Now()
	for _, fname := range l.idsFNames {
		utils.Logger.Debug("load ids from file", zap.String("fname", fname))
		fp, err = os.Open(fname)
		defer fp.Close()
		if err != nil {
			return nil, errors.Wrapf(err, "try to open file `%v` got error", fname)
		}

		idsDecoder := NewIdsDecoder(fp)
		newIds, err = idsDecoder.ReadAllToBmap()
		if err != nil {
			return nil, errors.Wrapf(err, "try to read file `%v` got error", fname)
		}

		ids.Or(newIds)
	}

	utils.Logger.Info("load all ids done", zap.Float64("sec", time.Now().Sub(startTs).Seconds()))
	return ids, nil
}

func (l *LegacyLoader) Clean() (err error) {
	l.ctx.dataFp.Close()

	if l.dataFNames != nil {
		for _, f := range l.dataFNames {
			utils.Logger.Info("remove data file", zap.String("f", f))
			if err = os.Remove(f); err != nil {
				utils.Logger.Error("try to delete file got error", zap.String("f", f), zap.Error(err))
			}
		}
		l.dataFNames = nil
	}

	if l.idsFNames != nil {
		for _, f := range l.idsFNames {
			utils.Logger.Info("remove id file", zap.String("f", f))
			if err = os.Remove(f); err != nil {
				utils.Logger.Error("try to delete file got error", zap.String("f", f), zap.Error(err))
			}
		}
		l.idsFNames = nil
	}

	utils.Logger.Info("clean all legacy")
	return err
}
