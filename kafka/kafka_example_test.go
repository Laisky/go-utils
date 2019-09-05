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
	cli, err := kafka.NewKafkaCliWithGroupId(context.Background(), &kafka.KafkaCliCfg{
		Brokers:          []string{"brokers url here"},
		Topics:           []string{"topics name here"},
		Groupid:          "group id",
		KMsgPool:         kmsgPool,
		IntervalNum:      100,
		IntervalDuration: 5 * time.Second,
	})
	if err != nil {
		panic(errors.Wrap(err, "try to connect to kafka got error"))
	}

	for kmsg := range cli.Messages(context.Background()) {
		// do something with kafka message
		fmt.Println(string(kmsg.Message))
		cli.CommitWithMsg(kmsg) // async commit
	}
}
