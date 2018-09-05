package journal

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type Data interface {
	GetId() int64
	SetId(int64)
	SetData(map[string]interface{})
}

type JournalConfig struct {
	BufDirPath   string
	BufSizeBytes int64
}

type Journal struct {
	cfg           *JournalConfig
	fsStat        *BufFileStat
	legacy        *LegacyLoader
	dataFp, idsFp *os.File
	dataEnc       *DataEncoder
	idsEnc        *IdsEncoder
	isRot         uint32
	i             uint64
}

func NewJournal(cfg *JournalConfig) *Journal {
	j := &Journal{
		cfg:   cfg,
		isRot: 0,
	}
	j.initBufDir()
	go j.runFlush()
	return j
}

// initBufDir initialize buf directory and create buf files
func (j *Journal) initBufDir() {
	err := PrepareDir(j.cfg.BufDirPath)
	if err != nil {
		panic(fmt.Errorf("call PrepareDir got error: %+v", err))
	}

	if err = j.Rotate(); err != nil {
		panic(err)
	}
}

func (j *Journal) Flush() (err error) {
	if j.idsEnc != nil {
		if err = j.idsEnc.Flush(); err != nil {
			err = errors.Wrap(err, "try to flush ids got error")
		}
	}

	if j.dataEnc != nil {
		if dataErr := j.dataEnc.Flush(); dataErr != nil {
			err = errors.Wrap(err, "try to flush data got error")
		}
	}

	return err
}

func (j *Journal) runFlush() {
	var (
		step = 1 * time.Second
		err  error
	)
	for {
		time.Sleep(step)
		if ok := atomic.CompareAndSwapUint32(&j.isRot, 0, 1); !ok {
			continue
		}

		if err = j.idsEnc.Flush(); err != nil {
			utils.Logger.Error("try to flush ids got error", zap.Error(err))
		}
		if err = j.dataEnc.Flush(); err != nil {
			utils.Logger.Error("try to flush data got error", zap.Error(err))
		}

		atomic.StoreUint32(&j.isRot, 0)
	}
}

func (j *Journal) checkRotate() error {
	j.i++
	if j.i > 100 {
		if fi, err := j.dataFp.Stat(); err != nil {
			return errors.Wrap(err, "try to load file stat got error")
		} else {
			if fi.Size() > j.cfg.BufSizeBytes {
				go j.Rotate()
				j.i = 0
				return DuringRotateErr
			}
		}
	}

	return nil
}

func (j *Journal) WriteData(data *map[string]interface{}) (err error) {
	if ok := atomic.CompareAndSwapUint32(&j.isRot, 0, 1); !ok {
		return DuringRotateErr
	}
	defer atomic.StoreUint32(&j.isRot, 0)

	if err = j.checkRotate(); err != nil {
		return err
	}

	return j.dataEnc.Write(data)
}

func (j *Journal) WriteId(id int64) error {
	if ok := atomic.CompareAndSwapUint32(&j.isRot, 0, 1); !ok {
		return DuringRotateErr
	}
	defer atomic.StoreUint32(&j.isRot, 0)

	return j.idsEnc.Write(id)
}

func (j *Journal) Rotate() (err error) {
	// this function should not concurrent
	for {
		if ok := atomic.CompareAndSwapUint32(&j.isRot, 0, 1); !ok {
			time.Sleep(1 * time.Millisecond)
			continue
		}
		break
	}
	defer atomic.StoreUint32(&j.isRot, 0)
	if err = j.Flush(); err != nil {
		return errors.Wrap(err, "try to flush journal got error")
	}

	// scan and creat files
	if j.fsStat, err = PrepareNewBufFile(j.cfg.BufDirPath); err != nil {
		return errors.Wrap(err, "call PrepareNewBufFile got error")
	}

	// create & open data file
	j.legacy = NewLegacyLoader(j.fsStat.OldDataFnames, j.fsStat.OldIdsDataFname)
	if j.dataFp != nil {
		j.dataFp.Close()
	}
	if j.dataFp, err = os.OpenFile(j.fsStat.NewDataFName, os.O_RDWR|os.O_CREATE, FileMode); err != nil {
		return errors.Wrap(err, "try to open data journal file got error")
	}
	utils.Logger.Info("create new data journal file", zap.String("file", j.fsStat.NewDataFName))
	j.dataEnc = NewDataEncoder(j.dataFp)

	// create & open ids file
	if j.idsFp != nil {
		j.idsFp.Close()
	}
	if j.idsFp, err = os.OpenFile(j.fsStat.NewIdsDataFname, os.O_RDWR|os.O_CREATE, FileMode); err != nil {
		return errors.Wrap(err, "try to open ids journal file got error")
	}
	utils.Logger.Info("create new ids journal file", zap.String("file", j.fsStat.NewIdsDataFname))
	j.idsEnc = NewIdsEncoder(j.idsFp)

	return nil
}

func (j *Journal) LoadLegacyBuf(data *map[string]interface{}) (err error) {
	if err = j.legacy.Load(data); err == io.EOF {
		j.legacy.Clean()
		return err
	} else if err != nil {
		return errors.Wrap(err, "load legacy journal got error")
	}

	return nil
}
