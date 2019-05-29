package kafka

import (
	"fmt"
	"sync"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/Shopify/sarama"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/pkg/errors"
)

type KafkaMsg struct {
	Topic     string
	Message   []byte
	Offset    int64
	Partition int32
	Timestamp time.Time
}

type KafkaCliCfg struct {
	Brokers, Topics  []string
	Groupid          string
	KMsgPool         *sync.Pool
	IntervalNum      int
	IntervalDuration time.Duration
}

type KafkaCli struct {
	*KafkaCliCfg
	cli                   *cluster.Consumer
	beforeChan, afterChan chan *KafkaMsg
}

func NewKafkaCliWithGroupId(cfg *KafkaCliCfg) (*KafkaCli, error) {
	utils.Logger.Debug("NewKafkaCliWithGroupId",
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
	cf := NewCommitFilter(&CommitFilterCfg{
		KMsgPool:         cfg.KMsgPool,
		IntervalNum:      cfg.IntervalNum,
		IntervalDuration: cfg.IntervalDuration,
	})

	// new KafkaCli
	k := &KafkaCli{
		KafkaCliCfg: cfg,
		cli:         consumer,
		beforeChan:  cf.GetBeforeChan(),
		afterChan:   cf.GetAfterChan(),
	}

	go k.ListenNotifications()
	go k.runCommitor()
	return k, nil
}

func (k *KafkaCli) Close() {
	k.cli.Close()
}

func (k *KafkaCli) ListenNotifications() {
	for ntf := range k.cli.Notifications() {
		// bugs: sarama-cluster's bug, will race for notification
		time.Sleep(50 * time.Millisecond)
		utils.Logger.Debug(fmt.Sprintf("KafkaCli Notify: %v", ntf))
	}
}

func (k *KafkaCli) Messages() <-chan *KafkaMsg {
	msgChan := make(chan *KafkaMsg, 100)
	var kmsg *KafkaMsg
	go func() {
		for msg := range k.cli.Messages() {
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

func (k *KafkaCli) runCommitor() {
	utils.Logger.Debug("start runCommitor")
	defer utils.Logger.Panic("kafka commitor exit")

	cmsg := &sarama.ConsumerMessage{}
	for kmsg := range k.afterChan {
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

func (k *KafkaCli) CommitWithMsg(kmsg *KafkaMsg) {
	k.beforeChan <- kmsg
}
