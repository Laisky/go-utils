package redis

import (
	"context"

	gredis "github.com/Laisky/go-redis"
	gutils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/eventengine/internal/consts"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
)

type Type struct {
	rdbKeyPrefix string
	rdb          *gredis.Utils
	logger       *gutils.LoggerType
}

type OptFunc func(t *Type) error

func WithRDBCli(rdb *redis.Client) OptFunc {
	return func(t *Type) error {
		if t == nil {
			return errors.Errorf("WithRDBCli's argument is nil")
		}

		t.rdb = gredis.NewRedisUtils(rdb)
		return nil
	}
}

func New(optfs ...OptFunc) (*Type, error) {
	t := new(Type)
	for _, optf := range optfs {
		if err := optf(t); err != nil {
			return nil, err
		}
	}

	if t.rdb == nil {
		t.rdb = gredis.NewRedisUtils(redis.NewClient(new(redis.Options)))
	}

	if t.logger == nil {
		t.logger = gutils.Logger.Named("regis")
	}

	return t, nil
}

func (t *Type) RDBKey() string {
	return t.rdbKeyPrefix + consts.RedisKeyQueue
}

func (t *Type) Put(ctx context.Context, evt *gutils.Event) error {
	msg, err := gutils.JSON.MarshalToString(evt)
	if err != nil {
		return err
	}

	return t.rdb.RPush(ctx, t.RDBKey(), msg)
}

func (t *Type) Get(ctx context.Context) (*gutils.Event, error) {
	_, v, err := t.rdb.LPopKeysBlocking(ctx, t.RDBKey())
	if err != nil {
		return nil, err
	}

	evt := new(gutils.Event)
	err = gutils.JSON.UnmarshalFromString(v, evt)
	return evt, err
}

// FIXME: not implement
func (t *Type) Commit(ctx context.Context, evt *gutils.Event) error {
	return errors.New("NotImplement")
}
