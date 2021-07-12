package eventengine

import (
	"github.com/Laisky/go-utils/eventengine/mq"
	"github.com/Laisky/go-utils/eventengine/mq/redis"
)

func WithRedisMQ(optfs ...redis.OptFunc) (mq.Interface, error) {
	return redis.New(optfs...)
}
