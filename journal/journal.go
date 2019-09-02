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
	deafultFlushInterval = 5 * time.Second
	// RotateCheckInterval interval to rotate journal files
	RotateCheckInterval   = 1 * time.Second
	defaultRotateDuration = 1 * time.Minute
	// defaultRotateDuration = 3 * time.Second // TODO
	defaultBufSizeBytes   = 1024 * 1024 * 200
	defaultCommittedIDTTL = 5 * time.Minute
)

// JournalConfig configuration of Journal
type JournalConfig struct {
	BufDirPath                string
	BufSizeBytes              int64
	RotateDuration            time.Duration
	IsAggresiveGC, IsCompress bool
	FlushInterval,            // interval to flush serializer
	CommittedIDTTL time.Duration
}

// NewConfig get JournalConfig with default configuration
func NewConfig() *JournalConfig {
	return &JournalConfig{
		RotateDuration: defaultRotateDuration,
		BufSizeBytes:   defaultBufSizeBytes,
		IsAggresiveGC:  true,
		IsCompress:     false,
		FlushInterval:  defaultBufSizeBytes,
		CommittedIDTTL: defaultCommittedIDTTL,
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
	latestRotateT          time.Time
	running                bool
}

// NewJournal create new Journal
func NewJournal(cfg *JournalConfig) *Journal {
	j := &Journal{
		JournalConfig: cfg,
		rotateLock:    utils.NewMutex(),
		legacyLock:    utils.NewMutex(),
		running:       true,
	}

	if j.RotateDuration < defaultRotateDuration {
		utils.Logger.Warn("journal rotate duration too short",
			zap.Duration("rotate_duration", j.RotateDuration))
	}
	if j.BufSizeBytes < 50*1024*1024 {
		utils.Logger.Warn("buf size bytes too small", zap.Int64("bytes", j.BufSizeBytes))
	}

	j.initBufDir()
	go j.runFlushTrigger()
	go j.runRotateTrigger()
	return j
}

func (j *Journal) Close() {
	j.Lock()
	j.running = false
	j.Flush()
	j.Unlock()
}

// initBufDir initialize buf directory and create buf files
func (j *Journal) initBufDir() {
	err := PrepareDir(j.BufDirPath)
	if err != nil {
		panic(fmt.Errorf("call PrepareDir got error: %+v", err))
	}

	if err = j.Rotate(); err != nil { // manually first run
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

// flushAndClose flush journal files
func (j *Journal) flushAndClose() (err error) {
	utils.Logger.Debug("flushAndClose")
	if j.idsEnc != nil {
		if err = j.idsEnc.Close(); err != nil {
			err = errors.Wrap(err, "try to close ids got error")
		}
	}

	if j.dataEnc != nil {
		if dataErr := j.dataEnc.Close(); dataErr != nil {
			err = errors.Wrap(err, "try to close data got error")
		}
	}

	return err
}

func (j *Journal) runFlushTrigger() {
	defer func() {
		if j.running {
			utils.Logger.Panic("journal flush exit")
		}
		utils.Logger.Info("journal flush exit")
	}()
	defer j.Flush()

	var err error
	for j.running {
		j.Lock()
		if err = j.Flush(); err != nil {
			utils.Logger.Error("try to flush ids&data got error", zap.Error(err))
		}
		j.Unlock()
		time.Sleep(j.FlushInterval)
	}
}

func (j *Journal) runRotateTrigger() {
	defer func() {
		if j.running {
			utils.Logger.Panic("journal rotate exit")
		}
		utils.Logger.Info("journal rotate exit")
	}()
	defer j.Flush()

	var err error
	for j.running {
		if j.isReadyToRotate() {
			if err = j.Rotate(); err != nil {
				utils.Logger.Error("try to rotate journal got error", zap.Error(err))
			}
		}

		time.Sleep(RotateCheckInterval)
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

func (j *Journal) isReadyToRotate() (ok bool) {
	if j.dataFp == nil {
		ok = true
	}
	// if !j.rotateLock.TryLock() {
	// 	utils.Logger.Debug("rotate not ready for cannot acquire rotateLock")
	// 	ok = false
	// }

	if fi, err := j.dataFp.Stat(); err != nil {
		utils.Logger.Error("try to get file stat got error", zap.Error(err))
		ok = false
	} else if fi.Size() > j.BufSizeBytes ||
		utils.Clock.GetUTCNow().Sub(j.latestRotateT) > j.RotateDuration {
		ok = true
	}

	utils.Logger.Debug("check isReadyToRotate",
		zap.Bool("ready", ok),
		zap.String("file", j.dataFp.Name()),
	)
	return
}

/*Rotate create new data and ids buf file
this function is not threadsafe.
*/
func (j *Journal) Rotate() (err error) {
	// make sure no other rorate is running
	if !j.rotateLock.TryLock() {
		return nil
	}
	defer j.rotateLock.ForceRealse()

	// stop legacy processing
	j.Lock()
	defer j.Unlock()
	utils.Logger.Debug("starting to rotate")

	if err = j.flushAndClose(); err != nil {
		return errors.Wrap(err, "try to flush journal got error")
	}

	j.latestRotateT = utils.Clock.GetUTCNow()
	// scan and create files
	if j.LockLegacy() {
		utils.Logger.Debug("acquired legacy lock, create new file and refresh legacy loader",
			zap.String("dir", j.BufDirPath))
		// need to refresh legacy, so need scan=true
		if j.fsStat, err = PrepareNewBufFile(j.BufDirPath, j.fsStat, true, j.IsCompress); err != nil {
			j.UnLockLegacy()
			return errors.Wrap(err, "call PrepareNewBufFile got error")
		}
		j.RefreshLegacyLoader()
		j.UnLockLegacy()
	} else {
		utils.Logger.Debug("can not acquire legacy lock, so only create new file",
			zap.String("dir", j.BufDirPath))
		// no need to scan old buf files
		if j.fsStat, err = PrepareNewBufFile(j.BufDirPath, j.fsStat, false, j.IsCompress); err != nil {
			return errors.Wrap(err, "call PrepareNewBufFile got error")
		}
	}

	// create & open data file
	if j.dataFp != nil {
		j.dataFp.Close()
	}
	j.dataFp = j.fsStat.NewDataFp
	if j.dataEnc, err = NewDataEncoder(j.dataFp, j.IsCompress); err != nil {
		return errors.Wrap(err, "try to create new data encoder got error")
	}

	// create & open ids file
	if j.idsFp != nil {
		j.idsFp.Close()
	}
	j.idsFp = j.fsStat.NewIDsFp
	if j.idsEnc, err = NewIdsEncoder(j.idsFp, j.IsCompress); err != nil {
		return errors.Wrap(err, "try to create new ids encoder got error")
	}

	return nil
}

// RefreshLegacyLoader create or reset legacy loader
func (j *Journal) RefreshLegacyLoader() {
	utils.Logger.Debug("RefreshLegacyLoader")
	if j.legacy == nil {
		j.legacy = NewLegacyLoader(j.fsStat.OldDataFnames, j.fsStat.OldIdsDataFname, j.IsCompress, j.CommittedIDTTL)
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
		"idsSetLen": j.legacy.GetIdsLen(),
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
