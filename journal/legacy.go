package journal

import (
	"io"
	"os"

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
	err = l.ctx.decoder.Read(data)
	if err == io.EOF {
		if l.ctx.dataFileIdx == l.ctx.dataFileMaxIdx { // all data files finished
			utils.Logger.Debug("all data files finished")
			return io.EOF
		}

		l.ctx.dataFp.Close()
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

	return nil
}

func (l *LegacyLoader) LoadAllids() (ids *roaring.Bitmap, err error) {
	utils.Logger.Debug("LoadAllids...")
	var (
		fp     *os.File
		newIds *roaring.Bitmap
	)
	ids = roaring.New()
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

	return ids, nil
}

func (l *LegacyLoader) Clean() (err error) {
	l.ctx.dataFp.Close()

	for _, f := range l.dataFNames {
		if err = os.Remove(f); err != nil {
			return errors.Wrapf(err, "try to delete `%v` got error", f)
		}
	}

	for _, f := range l.idsFNames {
		if err = os.Remove(f); err != nil {
			return errors.Wrapf(err, "try to delete `%v` got error", f)
		}
	}

	return nil
}
