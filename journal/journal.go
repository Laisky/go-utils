package journal

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

const (
	// FlushInterval interval to flush serializer
	FlushInterval = 1 * time.Second
	// RotateCheckInterval interval to rotate journal files
	RotateCheckInterval   = 1 * time.Second
	defaultRotateDuration = 1 * time.Minute
	defaultBufSizeBytes   = 1024 * 1024 * 200
)

// JournalConfig configuration of Journal
type JournalConfig struct {
	BufDirPath     string
	BufSizeBytes   int64
	RotateDuration time.Duration
	IsAggresiveGC  bool
}

// NewConfig get JournalConfig with default configuration
func NewConfig() *JournalConfig {
	return &JournalConfig{
		RotateDuration: defaultRotateDuration,
		BufSizeBytes:   defaultBufSizeBytes,
		IsAggresiveGC:  true,
	}
}

// Journal redo log consist by msgs and committed ids
type Journal struct {
	sync.RWMutex // journal rwlock
	*JournalConfig
	rotateLock, legacyLock *utils.Mutex
	dataFp, idsFp          *os.File // current writting journal file
	fsStat                 *BufFileStat
	legacy                 *LegacyLoader
	dataEnc                *DataEncoder
	idsEnc                 *IdsEncoder
	rotateCheckCnt         int
	latestRotateT          time.Time
}

// NewJournal create new Journal
func NewJournal(cfg *JournalConfig) *Journal {
	j := &Journal{
		JournalConfig: cfg,
		rotateLock:    utils.NewMutex(),
		legacyLock:    utils.NewMutex(),
	}

	if j.RotateDuration < 1*time.Minute {
		j.RotateDuration = 1 * time.Minute
	}
	if j.BufSizeBytes < 50*1024*1024 {
		utils.Logger.Warn("buf size bytes too small", zap.Int64("bytes", j.BufSizeBytes))
	}

	j.initBufDir()
	go j.runFlushTrigger()
	go j.runRotateTrigger()
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

// Flush flush journal files
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

func (j *Journal) runFlushTrigger() {
	defer utils.Logger.Panic("journal flush exit")
	var err error
	for {
		time.Sleep(FlushInterval)
		j.Lock()
		if err = j.Flush(); err != nil {
			utils.Logger.Error("try to flush ids&data got error", zap.Error(err))
		}
		j.Unlock()
	}
}

func (j *Journal) runRotateTrigger() {
	defer utils.Logger.Panic("journal rotate exit")
	for {
		time.Sleep(RotateCheckInterval)
		if j.dataFp == nil {
			continue
		}
		if fi, err := j.dataFp.Stat(); err != nil {
			continue
		} else {
			if fi.Size() > j.BufSizeBytes || utils.Clock.GetUTCNow().Sub(j.latestRotateT) > j.RotateDuration {
				go j.Rotate()
				j.rotateCheckCnt = 0
			}
		}
	}
}

// LoadMaxId load max id from journal ids files
func (j *Journal) LoadMaxId() (int64, error) {
	return j.legacy.LoadMaxId()
}

// WriteData write data to journal
func (j *Journal) WriteData(data *Data) (err error) {
	j.RLock() // will blocked by flush & rotate
	defer j.RUnlock()

	// utils.Logger.Debug("write data", zap.Int64("id", GetId(*data)))
	return j.dataEnc.Write(data)
}

// WriteId write id to journal
func (j *Journal) WriteId(id int64) error {
	j.RLock() // will blocked by flush & rotate
	defer j.RUnlock()

	j.legacy.AddID(id)
	return j.idsEnc.Write(id)
}

// Rotate create new data and ids buf file
// this function is not threadsafe
func (j *Journal) Rotate() (err error) {
	utils.Logger.Debug("try to rotate")

	if !j.rotateLock.TryLock() { // another rotate is running
		return
	}
	defer j.rotateLock.ForceRealse()

	j.Lock()
	defer j.Unlock()
	utils.Logger.Debug("starting to rotate")

	if err = j.Flush(); err != nil {
		return errors.Wrap(err, "try to flush journal got error")
	}

	j.latestRotateT = utils.Clock.GetUTCNow()
	// scan and create files
	if j.LockLegacy() {
		// need to refresh legacy, so need scan=true
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
		if j.IsAggresiveGC {
			utils.TriggerGC()
		}
	}
}

// LockLegacy lock legacy to prevent rotate
func (j *Journal) LockLegacy() bool {
	utils.Logger.Debug("try to lock legacy")
	return j.legacyLock.TryLock()
}

// IsLegacyRunning check whether running legacy loading
func (j *Journal) IsLegacyRunning() bool {
	utils.Logger.Debug("IsLegacyRunning")
	return j.legacyLock.IsLocked()
}

// UnLockLegacy release legacy lock
func (j *Journal) UnLockLegacy() bool {
	utils.Logger.Debug("try to unlock legacy")
	return j.legacyLock.TryRelease()
}

// GetMetric monitor inteface
func (j *Journal) GetMetric() map[string]interface{} {
	return map[string]interface{}{
		"idsSetLen": j.legacy.ctx.ids.GetLen(),
	}
}

// LoadLegacyBuf load legacy data one by one
// ⚠️Warn: should call `j.LockLegacy()` before invoke this method
func (j *Journal) LoadLegacyBuf(data *Data) (err error) {
	if !j.IsLegacyRunning() {
		utils.Logger.Panic("should call `j.LockLegacy()` first")
	}
	j.RLock()
	defer j.RUnlock()

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
