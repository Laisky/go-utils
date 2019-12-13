package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/Shopify/sarama"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/pkg/errors"
)

const (
	defaultCommitCheckInterval = 3 * time.Second
	defaultCommitCheckNum      = 10000
	defaultCommitCheckChanSize = 10000
)

type commitCheckOption struct {
	commitCheckInterval time.Duration
	commitCheckNum,
	commitCheckChanSize int
}

// CommitFilterOptFunc option for CommitFilter
type CommitFilterOptFunc func(*commitCheckOption)

// WithCommitFilterCheckInterval set commit check interval
func WithCommitFilterCheckInterval(interval time.Duration) CommitFilterOptFunc {
	return func(opt *commitCheckOption) {
		opt.commitCheckInterval = interval
	}
}

// WithCommitFilterCheckNum set commit check num
func WithCommitFilterCheckNum(num int) CommitFilterOptFunc {
	return func(opt *commitCheckOption) {
		opt.commitCheckNum = num
	}
}

// WithCommitFilterCheckChanSize set commit check channel's size
func WithCommitFilterCheckChanSize(size int) CommitFilterOptFunc {
	return func(opt *commitCheckOption) {
		opt.commitCheckChanSize = size
	}
}

// KafkaMsg kafka message
type KafkaMsg struct {
	Topic     string
	Message   []byte
	Offset    int64
	Partition int32
	Timestamp time.Time
}

// KafkaCliCfg configuration for kafka message
type KafkaCliCfg struct {
	Brokers, Topics []string
	Groupid         string
	KMsgPool        *sync.Pool
}

// KafkaCli kafka consumer client
type KafkaCli struct {
	*KafkaCliCfg
	stopChan chan struct{}

	cli                   *cluster.Consumer
	beforeChan, afterChan chan *KafkaMsg
}

// NewKafkaCliWithGroupID create new kafka consumer
func NewKafkaCliWithGroupID(ctx context.Context, cfg *KafkaCliCfg, opts ...CommitFilterOptFunc) (*KafkaCli, error) {
	utils.Logger.Debug("NewKafkaCliWithGroupID",
		zap.Strings("brokers", cfg.Brokers),
		zap.Strings("topics", cfg.Topics),
		zap.String("groupid", cfg.Groupid))

	// init sarama kafka client
	config := cluster.NewConfig()
	config.Net.KeepAlive = 30 * time.Second
	config.Consumer.Return.Errors = true
	config.Group.Return.Notifications = true
	config.Consumer.Offsets.CommitInterval = 1 * time.Second
	consumer, err := cluster.NewConsumer(cfg.Brokers, cfg.Groupid, cfg.Topics, config)
	if err != nil {
		return nil, errors.Wrap(err, "create kafka consumer got error")
	}

	// new commit filter
	cf := NewCommitFilter(ctx, cfg.KMsgPool, opts...)

	// new KafkaCli
	k := &KafkaCli{
		KafkaCliCfg: cfg,
		cli:         consumer,
		stopChan:    make(chan struct{}),
		beforeChan:  cf.GetBeforeChan(),
		afterChan:   cf.GetAfterChan(),
	}

	go k.ListenNotifications(ctx)
	go k.runCommitor(ctx)
	return k, nil
}

// Close close kafka client
func (k *KafkaCli) Close() {
	k.stopChan <- struct{}{}
	k.cli.Close()
}

// ListenNotifications log kafka broker notify
func (k *KafkaCli) ListenNotifications(ctx context.Context) {
	defer utils.Logger.Debug("ListenNotifications exit")
	var (
		ok  bool
		ntf *cluster.Notification
	)
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.stopChan:
			return
		case ntf, ok = <-k.cli.Notifications():
			if !ok {
				return
			}
		}

		utils.Logger.Debug(fmt.Sprintf("KafkaCli Notify: %v", ntf))
	}
}

// Messages get kafka messages chan
func (k *KafkaCli) Messages(ctx context.Context) <-chan *KafkaMsg {
	msgChan := make(chan *KafkaMsg, 1000)
	var (
		ok   bool
		msg  *sarama.ConsumerMessage
		kmsg *KafkaMsg
	)
	go func() {
		defer utils.Logger.Debug("message consumer exit")
		for {
			select {
			case <-ctx.Done():
				return
			case <-k.stopChan:
				return
			case msg, ok = <-k.cli.Messages():
				if !ok {
					return
				}
			}

			kmsg = k.KMsgPool.Get().(*KafkaMsg)
			kmsg.Topic = msg.Topic
			kmsg.Message = msg.Value
			kmsg.Offset = msg.Offset
			kmsg.Partition = msg.Partition
			kmsg.Timestamp = msg.Timestamp
			msgChan <- kmsg
		}
	}()

	return msgChan
}

func (k *KafkaCli) runCommitor(ctx context.Context) {
	utils.Logger.Debug("start runCommitor")
	defer utils.Logger.Debug("kafka commitor exit")

	var (
		cmsg = &sarama.ConsumerMessage{}
		kmsg *KafkaMsg
	)
	for {
		select {
		case <-ctx.Done():
			return
		case <-k.stopChan:
			return
		case kmsg = <-k.afterChan:
		}

		if utils.Settings.GetBool("dry") {
			utils.Logger.Info("commit message",
				zap.Int32("partition", kmsg.Partition),
				zap.Int64("offset", kmsg.Offset))
			continue
		}

		utils.Logger.Debug("commit message",
			zap.Int32("partition", kmsg.Partition),
			zap.Int64("offset", kmsg.Offset))
		cmsg.Topic = kmsg.Topic
		cmsg.Partition = kmsg.Partition
		cmsg.Offset = kmsg.Offset
		k.KMsgPool.Put(kmsg)
		k.cli.MarkOffset(cmsg, "")
	}
}

// CommitWithMsg commit kafka message
func (k *KafkaCli) CommitWithMsg(kmsg *KafkaMsg) {
	k.beforeChan <- kmsg
}
