package kafka

import (
	"sync"
	"time"

	utils "github.com/Laisky/go-utils"
	"go.uber.org/zap"
)

type msgRecord struct {
	num         int
	lastCommitT time.Time
	lastOffset  int64
	lastMsg     *KafkaMsg
	isCommited  bool
}

type CommitFilterCfg struct {
	KMsgPool         *sync.Pool
	IntervalNum      int
	IntervalDuration time.Duration
}

type CommitFilter struct {
	*CommitFilterCfg
	beforeChan, afterChan chan *KafkaMsg
}

func NewCommitFilter(cfg *CommitFilterCfg) *CommitFilter {
	utils.Logger.Info("NewCommitFilter",
		zap.Duration("interval_duration", cfg.IntervalDuration),
		zap.Int("interval_num", cfg.IntervalNum))
	f := &CommitFilter{
		CommitFilterCfg: cfg,
		beforeChan:      make(chan *KafkaMsg, 1000),
		afterChan:       make(chan *KafkaMsg, 1000),
	}
	go f.runFilterBeforeChan()
	return f
}

func (f *CommitFilter) GetBeforeChan() chan *KafkaMsg {
	return f.beforeChan
}

func (f *CommitFilter) GetAfterChan() chan *KafkaMsg {
	return f.afterChan
}

// runFilterBeforeChan maintain a kmsgSlots that cache the latest kmsg record.
// invoke filterSlots2AfterChan in fixed frequency.
func (f *CommitFilter) runFilterBeforeChan() {
	utils.Logger.Debug("start runFilterBeforeChan")
	defer utils.Logger.Panic("runFilterBeforeChan quit")
	var (
		kmsgSlots    = map[int32]*msgRecord{}
		kmsg         *KafkaMsg
		record       *msgRecord
		ok           bool
		now          time.Time
		scanInterval = time.Second * 1
		lastScanT    = time.Now()
	)

	for kmsg = range f.beforeChan {
		// record not exists, create new record
		if record, ok = kmsgSlots[kmsg.Partition]; !ok {
			kmsgSlots[kmsg.Partition] = &msgRecord{
				lastCommitT: time.Now(),
				lastMsg:     kmsg,
				lastOffset:  kmsg.Offset,
				num:         1,
				isCommited:  false,
			}
			continue
		}

		// record already exists
		if kmsg.Offset <= record.lastOffset {
			// current kmsg's offset is smaller than exists record
			// discard current kmsg
			f.KMsgPool.Put(kmsg)
		} else {
			// current kmsg's offset is bigger than exists
			// discard old record
			if !record.isCommited {
				// only recycle uncommited msg at here,
				// let commitor to recycle commited msg
				f.KMsgPool.Put(kmsgSlots[kmsg.Partition].lastMsg)
			}

			record.lastMsg = kmsg
			record.lastOffset = kmsg.Offset
			record.isCommited = false
		}

		record.num++

		now = time.Now()
		if now.Sub(lastScanT) > scanInterval {
			f.filterSlots2AfterChan(now, kmsgSlots)
		}
	}
}

// filterSlots2AfterChan filter all records in kmsgSlots,
// put kmsg that match `IntervalDuration` and `IntervalNum` conditions
// into innerChan to commit to kafka.
func (f *CommitFilter) filterSlots2AfterChan(now time.Time, kmsgSlots map[int32]*msgRecord) {
	utils.Logger.Debug("run filterSlots2AfterChan", zap.Time("now", now))
	for _, record := range kmsgSlots {
		if !record.isCommited &&
			(record.num > f.IntervalNum || now.Sub(record.lastCommitT) > f.IntervalDuration) {
			if utils.Settings.GetBool("dry") {
				utils.Logger.Info("put msg into afterChan",
					zap.Time("last_commit_time", record.lastCommitT),
					zap.Int("num", record.num))
				continue
			}

			select {
			case f.afterChan <- record.lastMsg:
				record.lastCommitT = now
				record.num = 0
				record.isCommited = true
			default:
			}
		}
	}
}
