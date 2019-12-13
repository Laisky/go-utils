package kafka_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Laisky/go-utils/kafka"
	"github.com/pkg/errors"
)

func ExampleKafkaCli() {
	var (
		kmsgPool = &sync.Pool{
			New: func() interface{} {
				return &kafka.KafkaMsg{}
			},
		}
	)
	cli, err := kafka.NewKafkaCliWithGroupID(
		context.Background(),
		&kafka.KafkaCliCfg{
			Brokers:  []string{"brokers url here"},
			Topics:   []string{"topics name here"},
			Groupid:  "group id",
			KMsgPool: kmsgPool,
		},
		kafka.WithCommitFilterCheckInterval(5*time.Second),
		kafka.WithCommitFilterCheckNum(100),
	)
	if err != nil {
		panic(errors.Wrap(err, "try to connect to kafka got error"))
	}

	for kmsg := range cli.Messages(context.Background()) {
		// do something with kafka message
		fmt.Println(string(kmsg.Message))
		cli.CommitWithMsg(kmsg) // async commit
	}
}
