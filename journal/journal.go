package journal

import (
	"fmt"
	"io"
	"os"
	"sync"
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
	cfg             *JournalConfig
	fsStat          *BufFileStat
	legacy          *LegacyLoader
	dataFp, idsFp   *os.File
	dataEnc         *DataEncoder
	idsEnc          *IdsEncoder
	l               *sync.RWMutex
	isLegacyRunning uint32
	i               uint64
}

func NewJournal(cfg *JournalConfig) *Journal {
	j := &Journal{
		cfg:             cfg,
		isLegacyRunning: 0,
		l:               &sync.RWMutex{},
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
		utils.Logger.Debug("flush ids")
		if err = j.idsEnc.Flush(); err != nil {
			err = errors.Wrap(err, "try to flush ids got error")
		}
	}

	if j.dataEnc != nil {
		utils.Logger.Debug("flush data")
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
		j.l.Lock()
		if err = j.Flush(); err != nil {
			utils.Logger.Error("try to flush ids&data got error", zap.Error(err))
		}
		j.l.Unlock()
	}
}

func (j *Journal) checkRotate() error {
	j.i++
	if j.i > 1000 {
		if fi, err := j.dataFp.Stat(); err != nil {
			return errors.Wrap(err, "try to load file stat got error")
		} else {
			if fi.Size() > j.cfg.BufSizeBytes {
				go j.Rotate()
				j.i = 0
			}
		}
	}

	return nil
}

func (j *Journal) WriteData(data *map[string]interface{}) (err error) {
	j.l.RLock() // will blocked by flush & rotate
	defer j.l.RUnlock()

	if err = j.checkRotate(); err != nil {
		return err
	}

	return j.dataEnc.Write(data)
}

func (j *Journal) WriteId(id int64) error {
	j.l.RLock() // will blocked by flush & rotate
	defer j.l.RUnlock()

	return j.idsEnc.Write(id)
}

// Rotate create new data and ids buf file
// this function is not threadsafe
func (j *Journal) Rotate() (err error) {
	if !j.LockLegacy() {
		return
	}
	defer j.UnLockLegacy()

	j.l.Lock()
	defer j.l.Unlock()

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

func (j *Journal) LockLegacy() bool {
	utils.Logger.Debug("try to lock legacy")
	return atomic.CompareAndSwapUint32(&j.isLegacyRunning, 0, 1)
}

func (j *Journal) IsLegacyRunning() bool {
	utils.Logger.Debug("IsLegacyRunning")
	return atomic.LoadUint32(&j.isLegacyRunning) == 1
}

func (j *Journal) UnLockLegacy() bool {
	utils.Logger.Debug("try to unlock legacy")
	return atomic.CompareAndSwapUint32(&j.isLegacyRunning, 1, 0)
}

// LoadLegacyBuf load legacy data one by one
// should call LockLegacy first
func (j *Journal) LoadLegacyBuf(data *map[string]interface{}) (err error) {
	j.l.RLock()
	defer j.l.RUnlock()

	if err = j.legacy.Load(data); err == io.EOF {
		utils.Logger.Info("LoadLegacyBuf done")
		if err = j.legacy.Clean(); err != nil {
			utils.Logger.Error("clean buf files got error", zap.Error(err))
		}
		j.UnLockLegacy()
		return io.EOF
	} else if err != nil {
		j.UnLockLegacy()
		return errors.Wrap(err, "load legacy journal got error")
	}

	return nil
}
