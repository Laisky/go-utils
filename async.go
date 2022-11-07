package utils

import (
	"context"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v2/log"
)

// AsyncTaskStatus status of async task
type AsyncTaskStatus string

// String convert status to string
func (s AsyncTaskStatus) String() string {
	return string(s)
}

const (
	// AsyncTaskStatusPending task pending
	AsyncTaskStatusPending AsyncTaskStatus = "pending"
	// AsyncTaskStatusDone task done
	AsyncTaskStatusDone AsyncTaskStatus = "done"
	// AsyncTaskStatusFailed task failed
	AsyncTaskStatusFailed AsyncTaskStatus = "failed"
)

var (
	// ErrAsyncTask root error for async tasks
	ErrAsyncTask = errors.New("async task error")
)

// AsyncTaskResult result of async task
type AsyncTaskResult struct {
	TaskID string          `json:"task_id"`
	Status AsyncTaskStatus `json:"status"`
	Data   string          `json:"data"`
	Err    string          `json:"err"`
}

// AsyncTaskStore persistency storage for async task
type AsyncTaskStore interface {
	// New create new AsyncTaskResult with id
	New(ctx context.Context) (result *AsyncTaskResult, err error)
	// Set set AsyncTaskResult
	Set(ctx context.Context, taskID string, result *AsyncTaskResult) (err error)
	// Heartbeat refresh async task's updated time to mark this task is still alive
	Heartbeat(ctx context.Context, taskID string) (alived bool, err error)
}

// asyncTask async task
type AsyncTask interface {
	// ID get task id
	ID() string
	// Status get task status, pending/done/failed
	Status() AsyncTaskStatus
	// SetDone set task done with result data
	SetDone(ctx context.Context, data string) (err error)
	// SetError set task error with err message
	SetError(ctx context.Context, errMsg string) (err error)
}

type asyncTask struct {
	id     string
	store  AsyncTaskStore
	result *AsyncTaskResult
	cancel func()
}

// NewTask new async task
func NewAsyncTask(ctx context.Context, store AsyncTaskStore) (AsyncTask, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-ctx.Done()
		cancel()
	}()

	result, err := store.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "new async task result")
	}

	result.Status = AsyncTaskStatusPending
	t := &asyncTask{
		id:     result.TaskID,
		store:  store,
		result: result,
		cancel: cancel,
	}

	if err := store.Set(ctx, t.id, t.result); err != nil {
		return nil, errors.Wrap(err, "set async task result")
	}

	go t.heartbeat(ctx)
	return t, nil
}

func (t *asyncTask) heartbeat(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if alived, err := t.store.Heartbeat(ctx, t.id); err != nil {
			log.Shared.Error("async task heartbeat", zap.Error(err))
		} else if !alived {
			return
		}

		SleepWithContext(ctx, 10*time.Second)
	}
}

// ID get task id
func (t *asyncTask) ID() string {
	return t.id
}

// Status get task status
func (t *asyncTask) Status() AsyncTaskStatus {
	return t.result.Status
}

// SetDone set task done with result data
func (t *asyncTask) SetDone(ctx context.Context, data string) (err error) {
	if t.result.Status != AsyncTaskStatusPending {
		return errors.Errorf("task already %s", t.result.Status.String())
	}

	defer t.cancel()
	t.result.Status = AsyncTaskStatusDone
	t.result.Data = data

	if err = t.store.Set(ctx, t.id, t.result); err != nil {
		return errors.Wrapf(err, "set async task `%s` done", t.id)
	}

	return nil
}

// SetError set task error with err message
func (t *asyncTask) SetError(ctx context.Context, errMsg string) (err error) {
	if t.result.Status != AsyncTaskStatusPending {
		return errors.Errorf("task already %s", t.result.Status.String())
	}

	defer t.cancel()
	t.result.Status = AsyncTaskStatusFailed
	t.result.Err = errMsg

	if err = t.store.Set(ctx, t.id, t.result); err != nil {
		return errors.Wrapf(err, "set async task `%s` failed", t.id)
	}

	return nil
}
