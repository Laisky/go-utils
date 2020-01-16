package journal

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

// LegacyLoader loader to handle legacy data and ids
type LegacyLoader struct {
	sync.Mutex

	dataFNames, idsFNames []string
	isNeedReload,         // prepare datafp for `Load`
	isCompress,
	isReadyReload bool // alreddy update `dataFNames`
	ids                       Int64SetItf
	dataFileIdx, dataFilesLen int
	dataFp                    *os.File
	decoder                   *DataDecoder
}

// NewLegacyLoader create new LegacyLoader
func NewLegacyLoader(ctx context.Context, dataFNames, idsFNames []string, isCompress bool, committedIDTTL time.Duration) *LegacyLoader {
	utils.Logger.Debug("new legacy loader", zap.Strings("dataFiles", dataFNames), zap.Strings("idsFiles", idsFNames))
	l := &LegacyLoader{
		dataFNames:    dataFNames,
		idsFNames:     idsFNames,
		isNeedReload:  true,
		isReadyReload: len(dataFNames) != 0,
		isCompress:    isCompress,
		ids:           NewInt64SetWithTTL(ctx, committedIDTTL),
	}

	return l
}

// AddID add id in ids
func (l *LegacyLoader) AddID(id int64) {
	l.ids.AddInt64(id)
}

func (l *LegacyLoader) IsIDExists(id int64) bool {
	return l.ids.CheckAndRemove(id)
}

// Reset reset journal legacy link to existing files
func (l *LegacyLoader) Reset(dataFNames, idsFNames []string) {
	l.Lock()
	defer l.Unlock()

	utils.Logger.Debug("reset legacy loader", zap.Strings("dataFiles", dataFNames), zap.Strings("idsFiles", idsFNames))
	l.dataFNames = dataFNames
	l.idsFNames = idsFNames
	// l.ids = NewInt64Set()
	l.isReadyReload = len(dataFNames) != 0
}

// GetIdsLen return length of ids
func (l *LegacyLoader) GetIdsLen() int {
	l.Lock()
	defer l.Unlock()
	return l.ids.GetLen()
}

// removeFile delete file, should run sync to avoid dirty files
func (l *LegacyLoader) removeFiles(fs []string) {
	for _, fpath := range fs {

		if err := os.Remove(fpath); err != nil {
			utils.Logger.Error("delete file",
				zap.String("file", fpath),
				zap.Error(err))
		}
		utils.Logger.Info("remove buf file", zap.String("file", fpath))
	}
}

func (l *LegacyLoader) Load(data *Data) (err error) {
	utils.Logger.Debug("load legacy msg",
		zap.Bool("isNeedReload", l.isNeedReload),
		zap.Bool("isReadyReload", l.isReadyReload),
	)

	if l.isNeedReload { // reload ctx
		if !l.isReadyReload { // legacy files not prepared
			return io.EOF
		}
		l.isReadyReload = false

		utils.Logger.Debug("reload ids & data file idx")
		if err = l.LoadAllids(l.ids); err != nil {
			utils.Logger.Error("load all ids", zap.Error(err))
		}
		l.dataFilesLen = len(l.dataFNames) - 1
		l.dataFileIdx = -1
		l.isNeedReload = false
	}

READ_NEW_FILE:
	if l.dataFp == nil {
		l.dataFileIdx++
		if l.dataFileIdx == l.dataFilesLen { // all data files finished
			utils.Logger.Debug("all data files finished")
			l.isNeedReload = true
			return io.EOF
		}

		utils.Logger.Debug("read new data file",
			zap.Strings("data_files", l.dataFNames),
			zap.String("fname", l.dataFNames[l.dataFileIdx]))
		l.dataFp, err = os.Open(l.dataFNames[l.dataFileIdx])
		if err != nil {
			utils.Logger.Error("open data file", zap.Error(err))
			l.dataFp = nil
			goto READ_NEW_FILE
		}

		if l.decoder, err = NewDataDecoder(l.dataFp, isFileGZ(l.dataFp.Name())); err != nil {
			utils.Logger.Error("decode data file", zap.Error(err))
			l.dataFp = nil
			goto READ_NEW_FILE
		}
	}

READ_NEW_LINE:
	if err = l.decoder.Read(data); err != nil {
		if err != io.EOF {
			// current file is broken
			utils.Logger.Error("load data file", zap.Error(err))
		}

		// read new file
		if err = l.dataFp.Close(); err != nil {
			utils.Logger.Error("close file", zap.String("file", l.dataFNames[l.dataFileIdx]), zap.Error(err))
		}

		l.dataFp = nil
		utils.Logger.Debug("finish read file", zap.String("fname", l.dataFNames[l.dataFileIdx]))
		goto READ_NEW_FILE
	}

	if l.ids.CheckAndRemove(data.ID) { // ignore committed data
		// utils.Logger.Debug("data already consumed", zap.Int64("id", id))
		goto READ_NEW_LINE
	}

	// utils.Logger.Debug("load unconsumed data", zap.Int64("id", id))
	return nil
}

// LoadMaxId load max id from all ids files
func (l *LegacyLoader) LoadMaxId() (maxId int64, err error) {
	utils.Logger.Debug("LoadMaxId...")
	var (
		fp         *os.File
		id         int64
		idsDecoder *IdsDecoder
	)
	startTs := utils.Clock.GetUTCNow()
	for _, fname := range l.idsFNames {
		// utils.Logger.Debug("load ids from file", zap.String("fname", fname))
		fp, err = os.Open(fname)
		if err != nil {
			return 0, errors.Wrapf(err, "open file `%v` to load maxid", fname)
		}
		defer fp.Close()

		if idsDecoder, err = NewIdsDecoder(fp, isFileGZ(fp.Name())); err != nil {
			utils.Logger.Error("read ids file",
				zap.Error(err),
				zap.String("fname", fp.Name()),
			)
			continue
		}
		if id, err = idsDecoder.LoadMaxId(); err != nil {
			utils.Logger.Error("read ids file",
				zap.Error(err),
				zap.String("fname", fp.Name()),
			)
			continue
		}
		if id < maxId {
			maxId = id
		}
	}

	utils.Logger.Debug("load max id done", zap.Float64("sec", utils.Clock.GetUTCNow().Sub(startTs).Seconds()))
	return id, nil
}

// LoadAllids read all ids from ids file into ids set
func (l *LegacyLoader) LoadAllids(ids Int64SetItf) (allErr error) {
	utils.Logger.Debug("LoadAllids...")
	var (
		err        error
		fp         *os.File
		idsDecoder *IdsDecoder
	)

	defer func() {
		if fp != nil {
			fp.Close()
		}
		if allErr != nil {
			utils.Logger.Error("load all ids", zap.Error(allErr))
		}
	}()

	startTs := utils.Clock.GetUTCNow()
	for _, fname := range l.idsFNames {
		// utils.Logger.Debug("load ids from file", zap.String("fname", fname))
		if fp != nil {
			fp.Close()
		}
		fp, err = os.Open(fname)
		if err != nil {
			allErr = errors.Wrapf(err, "open ids file `%v` to load all ids", fname)
			continue
		}
		if idsDecoder, err = NewIdsDecoder(fp, isFileGZ(fp.Name())); err != nil {
			allErr = errors.Wrapf(err, "decode ids file `%v`", fname)
			continue
		}
		if err = idsDecoder.ReadAllToInt64Set(ids); err != nil {
			allErr = errors.Wrapf(err, "read ids file `%v`", fname)
			continue
		}
	}

	utils.Logger.Debug("load all ids done", zap.Float64("sec", utils.Clock.GetUTCNow().Sub(startTs).Seconds()))
	return allErr
}

// Clean clean legacy files
func (l *LegacyLoader) Clean() error {
	if len(l.dataFNames) > 1 {
		l.removeFiles(l.dataFNames[:len(l.dataFNames)-1])
		l.dataFNames = []string{l.dataFNames[len(l.dataFNames)-1]}
	}
	if len(l.idsFNames) > 1 {
		l.removeFiles(l.idsFNames[:len(l.idsFNames)-1])
		l.idsFNames = []string{l.idsFNames[len(l.idsFNames)-1]}
	}

	l.dataFp.Close()
	l.dataFp = nil // `Load` need this
	utils.Logger.Debug("clean all legacy files")
	return nil
}
