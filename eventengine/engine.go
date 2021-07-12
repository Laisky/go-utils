package eventengine

import (
	"eventengine/mq"
	"eventengine/mq/redis"
)

func WithRedisMQ(optfs ...redis.OptFunc) (mq.Interface, error) {
	return redis.New(optfs...)
}
