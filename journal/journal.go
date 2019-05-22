package journal

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

type JournalConfig struct {
	BufDirPath             string
	BufSizeBytes           int64
	RotateCheckIntervalNum int
	RotateDuration         time.Duration
}

type Journal struct {
	*JournalConfig
	dataFp, idsFp   *os.File // current writting journal file
	fsStat          *BufFileStat
	legacy          *LegacyLoader
	dataEnc         *DataEncoder
	idsEnc          *IdsEncoder
	l               *sync.RWMutex // journal rwlock
	isLegacyRunning uint32        // true if is loading legacy now
	rotateCheckCnt  int
	latestRotateT   time.Time
}

func NewJournal(cfg *JournalConfig) *Journal {
	j := &Journal{
		JournalConfig:   cfg,
		isLegacyRunning: 0,
		l:               &sync.RWMutex{},
	}

	if j.RotateCheckIntervalNum <= 0 {
		j.RotateCheckIntervalNum = 1000
	}

	if j.RotateDuration < 1*time.Minute {
		j.RotateDuration = 1 * time.Minute
	}

	j.initBufDir()
	go j.runFlush()
	return j
}

// initBufDir initialize buf directory and create buf files
func (j *Journal) initBufDir() {
	err := PrepareDir(j.BufDirPath)
	if err != nil {
		panic(fmt.Errorf("call PrepareDir got error: %+v", err))
	}

	if err = j.Rotate(); err != nil {
		panic(err)
	}
}

func (j *Journal) Flush() (err error) {
	if j.idsEnc != nil {
		// utils.Logger.Debug("flush ids")
		if err = j.idsEnc.Flush(); err != nil {
			err = errors.Wrap(err, "try to flush ids got error")
		}
	}

	if j.dataEnc != nil {
		// utils.Logger.Debug("flush data")
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
	if fi, err := j.dataFp.Stat(); err != nil {
		return errors.Wrap(err, "try to load file stat got error")
	} else {
		if fi.Size() > j.BufSizeBytes || utils.Clock.GetUTCNow().Sub(j.latestRotateT) > j.RotateDuration {
			go j.Rotate()
			j.rotateCheckCnt = 0
		}
	}

	return nil
}

func (j *Journal) LoadMaxId() (int64, error) {
	return j.legacy.LoadMaxId()
}

func (j *Journal) WriteData(data *Data) (err error) {
	j.l.RLock() // will blocked by flush & rotate
	defer j.l.RUnlock()

	j.rotateCheckCnt++
	if j.rotateCheckCnt > j.RotateCheckIntervalNum {
		if err = j.checkRotate(); err != nil {
			return errors.Wrap(err, "check rotate got error")
		}
	}

	// utils.Logger.Debug("write data", zap.Int64("id", GetId(*data)))
	return j.dataEnc.Write(data)
}

func (j *Journal) WriteId(id int64) error {
	j.l.RLock() // will blocked by flush & rotate
	defer j.l.RUnlock()

	j.legacy.AddID(id)
	return j.idsEnc.Write(id)
}

// Rotate create new data and ids buf file
// this function is not threadsafe
func (j *Journal) Rotate() (err error) {
	utils.Logger.Debug("try to rotate")
	j.l.Lock()
	defer j.l.Unlock()
	utils.Logger.Debug("starting to rotate")

	if err = j.Flush(); err != nil {
		return errors.Wrap(err, "try to flush journal got error")
	}

	j.latestRotateT = utils.Clock.GetUTCNow()
	// scan and create files
	if j.LockLegacy() {
		if j.fsStat, err = PrepareNewBufFile(j.BufDirPath, j.fsStat, true); err != nil {
			j.UnLockLegacy()
			return errors.Wrap(err, "call PrepareNewBufFile got error")
		}
		j.RefreshLegacyLoader()
		j.UnLockLegacy()
	} else {
		// no need to scan old buf files
		if j.fsStat, err = PrepareNewBufFile(j.BufDirPath, j.fsStat, false); err != nil {
			return errors.Wrap(err, "call PrepareNewBufFile got error")
		}
	}

	// create & open data file
	if j.dataFp != nil {
		j.dataFp.Close()
	}
	j.dataFp = j.fsStat.NewDataFp
	j.dataEnc = NewDataEncoder(j.dataFp)

	// create & open ids file
	if j.idsFp != nil {
		j.idsFp.Close()
	}
	j.idsFp = j.fsStat.NewIDsFp
	j.idsEnc = NewIdsEncoder(j.idsFp)

	return nil
}

// RefreshLegacyLoader create or reset legacy loader
func (j *Journal) RefreshLegacyLoader() {
	utils.Logger.Debug("RefreshLegacyLoader")
	if j.legacy == nil {
		j.legacy = NewLegacyLoader(j.fsStat.OldDataFnames, j.fsStat.OldIdsDataFname)
	} else {
		j.legacy.Reset(j.fsStat.OldDataFnames, j.fsStat.OldIdsDataFname)
	}
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
// ⚠️Warn: should call `j.LockLegacy()` before invoke this method
func (j *Journal) LoadLegacyBuf(data *Data) (err error) {
	j.l.RLock()
	defer j.l.RUnlock()

	if err = j.legacy.Load(data); err == io.EOF {
		utils.Logger.Debug("LoadLegacyBuf done")
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
