package redis

import (
	"context"

	gredis "github.com/Laisky/go-redis"
	gutils "github.com/Laisky/go-utils"
	"github.com/go-redis/redis/v8"
)

type Type struct {
	rdbKeyPrefix string
	rdb          *gredis.Utils
	logger       *gutils.LoggerType
}

func New(rdb *redis.Client) (*Type, error) {
	t := &Type{
		logger: gutils.Logger.Named("regis"),
		rdb:    gredis.NewRedisUtils(rdb),
	}

	return t, nil
}

func (t *Type) Put(ctx context.Context, evt *gutils.Event) error {
	msg, err := gutils.JSON.MarshalToString(evt)
	if err != nil {
		return err
	}

	return t.rdb.RPush(ctx, t.rdbKeyPrefix+"queue/", msg)
}

func (t *Type) Get(ctx context.Context) (*gutils.Event, error) {
	_, v, err := t.rdb.LPopKeysBlocking(ctx, t.rdbKeyPrefix+"queue/")
	if err != nil {
		return nil, err
	}

	gutils.JSON.

}
