package mq

import (
	"context"

	gutils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/eventengine/mq/redis"
)

var (
	_ Interface = new(redis.Type)
)

type Interface interface {
	Put(ctx context.Context, evt *gutils.Event) error
	Commit(ctx context.Context, evt *gutils.Event) error
	Get(ctx context.Context) (evt *gutils.Event, err error)
}
