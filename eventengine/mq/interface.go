package mq

import (
	"context"
	"eventengine/mq/redis"

	gutils "github.com/Laisky/go-utils"
)

var (
	_ Interface = new(redis.Type)
)

type Interface interface {
	Put(ctx context.Context, evt *gutils.Event) error
	Commit(ctx context.Context, evt *gutils.Event) error
	Get(ctx context.Context) (evt *gutils.Event, err error)
}
