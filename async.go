package utils

import (
	"context"
	"sync"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v4/log"
)

// AsyncTaskStatus status of async task
type AsyncTaskStatus uint

// String convert status to string
func (s AsyncTaskStatus) String() string {
	switch s {
	case AsyncTaskStatusPending:
		return "pending"
	case AsyncTaskStatusDone:
		return "done"
	case AsyncTaskStatusFailed:
		return "failed"
	default:
		return "unspecified"
	}
}

const (
	// AsyncTaskStatusUnspecified unknown
	AsyncTaskStatusUnspecified AsyncTaskStatus = iota
	// AsyncTaskStatusPending task pending
	AsyncTaskStatusPending
	// AsyncTaskStatusDone task done
	AsyncTaskStatusDone
	// AsyncTaskStatusFailed task failed
	AsyncTaskStatusFailed
)

var (
	// ErrAsyncTask root error for async tasks
	ErrAsyncTask = errors.New("async task error")

	_ AsyncTaskInterface = new(AsyncTask)
)

// AsyncTaskResult result of async task
type AsyncTaskResult struct {
	TaskID string          `json:"task_id"`
	Status AsyncTaskStatus `json:"status"`
	Data   string          `json:"data"`
	Err    string          `json:"err"`
}

// AsyncTaskStoreInterface persistency storage for async task
type AsyncTaskStoreInterface interface {
	// New create new AsyncTaskResult with id
	New(ctx context.Context) (result *AsyncTaskResult, err error)
	// Set AsyncTaskResult
	Set(ctx context.Context, taskID string, result *AsyncTaskResult) (err error)
	// Heartbeat refresh async task's updated time to mark this task is still alive
	Heartbeat(ctx context.Context, taskID string) (alived bool, err error)
	// Get task by id
	Get(ctx context.Context, taskID string) (result *AsyncTaskResult, err error)
	// Delete task by id
	Delete(ctx context.Context, taskID string) (err error)
}

// AsyncTaskStoreMemory example store in memory
type AsyncTaskStoreMemory struct {
	store sync.Map
}

// NewAsyncTaskStoreMemory new default memory store
func NewAsyncTaskStoreMemory() *AsyncTaskStoreMemory {
	return &AsyncTaskStoreMemory{
		store: sync.Map{},
	}
}

// New create new AsyncTaskResult with id
func (s *AsyncTaskStoreMemory) New(ctx context.Context) (result *AsyncTaskResult, err error) {
	t := &AsyncTaskResult{
		TaskID: UUID1(),
		Status: AsyncTaskStatusPending,
	}
	s.store.Store(t.TaskID, t)
	return t, nil
}

// Get get task by id
func (s *AsyncTaskStoreMemory) Get(ctx context.Context, taskID string) (result *AsyncTaskResult, err error) {
	ri, ok := s.store.Load(taskID)
	if !ok {
		return nil, errors.Errorf("task %q notfound", taskID)
	}

	return ri.(*AsyncTaskResult), nil
}

// Delete task by id
func (s *AsyncTaskStoreMemory) Delete(ctx context.Context, taskID string) (err error) {
	s.store.Delete(taskID)
	return nil
}

// Set set AsyncTaskResult
func (s *AsyncTaskStoreMemory) Set(ctx context.Context, taskID string, result *AsyncTaskResult) (err error) {
	s.store.Store(taskID, result)
	return nil
}

// Heartbeat refresh async task's updated time to mark this task is still alive
func (s *AsyncTaskStoreMemory) Heartbeat(ctx context.Context, taskID string) (alived bool, err error) {
	return true, nil
}

// asyncTask async task
type AsyncTaskInterface interface {
	// ID get task id
	ID() string
	// Status get task status, pending/done/failed
	Status() AsyncTaskStatus
	// SetDone set task done with result data
	SetDone(ctx context.Context, data string) (err error)
	// SetError set task error with err message
	SetError(ctx context.Context, errMsg string) (err error)
}

// AsyncTask async task manager
type AsyncTask struct {
	id     string
	store  AsyncTaskStoreInterface
	result *AsyncTaskResult
	cancel func()
}

// NewTask new async task
//
// ctx must keep alive for whole lifecycle of AsyncTask
func NewAsyncTask(ctx context.Context, store AsyncTaskStoreInterface) (
	*AsyncTask, error) {
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
	t := &AsyncTask{
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

func (t *AsyncTask) heartbeat(ctx context.Context) {
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
func (t *AsyncTask) ID() string {
	return t.id
}

// Status get task status
func (t *AsyncTask) Status() AsyncTaskStatus {
	return t.result.Status
}

// SetDone set task done with result data
func (t *AsyncTask) SetDone(ctx context.Context, data string) (err error) {
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
func (t *AsyncTask) SetError(ctx context.Context, errMsg string) (err error) {
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
