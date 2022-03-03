package utils

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

type AsyncTaskStatus string

const (
	// AsyncTaskStatusPending task pending
	AsyncTaskStatusPending AsyncTaskStatus = "pending"
	// AsyncTaskStatusDone task done
	AsyncTaskStatusDone AsyncTaskStatus = "done"
	// AsyncTaskStatusFailed task failed
	AsyncTaskStatusFailed AsyncTaskStatus = "failed"
)

var (
	ErrAsyncTask = errors.New("async task error")
)

// AsyncTaskResult result of async task
type AsyncTaskResult struct {
	TaskID string          `json:"task_id"`
	Status AsyncTaskStatus `json:"status"`
	Exp    time.Time       `json:"exp"`
	Data   string          `json:"data"`
	Err    string          `json:"err"`
}

// AsyncTaskStore persistency storage for async task
type AsyncTaskStore interface {
	// New create new AsyncTaskResult with id
	New(ctx context.Context) (result *AsyncTaskResult, err error)
	// Set set AsyncTaskResult
	Set(ctx context.Context, taskID string, result *AsyncTaskResult) (err error)
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
}

// NewTask new async task
func NewAsyncTask(ctx context.Context, store AsyncTaskStore) (AsyncTask, error) {
	result, err := store.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "new async task result")
	}

	result.Exp = Clock.GetUTCNow().Add(time.Hour)
	result.Status = AsyncTaskStatusPending
	t := &asyncTask{
		id:     result.TaskID,
		store:  store,
		result: result,
	}

	if err := store.Set(ctx, t.id, t.result); err != nil {
		return nil, errors.Wrap(err, "set async task result")
	}

	return t, nil
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
	if t.result.Exp.After(Clock.GetUTCNow()) {
		return errors.Wrap(ErrAsyncTask, "task expired")
	}

	t.result.Status = AsyncTaskStatusDone
	t.result.Data = data

	if err = t.store.Set(ctx, t.id, t.result); err != nil {
		return errors.Wrapf(err, "set async task `%s` done", t.id)
	}

	return nil
}

// SetError set task error with err message
func (t *asyncTask) SetError(ctx context.Context, errMsg string) (err error) {
	if t.result.Exp.After(Clock.GetUTCNow()) {
		return errors.Wrap(ErrAsyncTask, "task expired")
	}

	t.result.Status = AsyncTaskStatusFailed
	t.result.Err = errMsg

	if err = t.store.Set(ctx, t.id, t.result); err != nil {
		return errors.Wrapf(err, "set async task `%s` failed", t.id)
	}

	return nil
}
