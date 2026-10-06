package log

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// shutdownSecurityFormatter pauses after the hook's initial lifecycle check.
type shutdownSecurityFormatter struct{ entered, release chan struct{} }

// Format resumes only after the fixture has completed worker shutdown.
func (f *shutdownSecurityFormatter) Format(zapcore.Entry, []zapcore.Field) ([]byte, error) {
	close(f.entered)
	<-f.release
	return []byte("synthetic shutdown entry"), nil
}

// TestSecurity63NoAdmissionAfterShutdown proves that a formatter already running
// cannot admit an entry after Close or parent cancellation has stopped the worker.
func TestSecurity63NoAdmissionAfterShutdown(t *testing.T) {
	for _, mode := range []string{"close", "parent"} {
		t.Run(mode, func(t *testing.T) {
			admitted := 0
			for range 32 {
				ctx, cancel := context.WithCancel(context.Background())
				formatter := &shutdownSecurityFormatter{make(chan struct{}), make(chan struct{})}
				p, err := NewPusher(ctx, WithPusherFormatter(formatter), WithPusherSenderChanLen(1))
				require.NoError(t, err)
				result := make(chan error, 1)
				go func() { result <- p.GetZapHook()(zapcore.Entry{}, nil) }()
				<-formatter.entered
				if mode == "close" {
					p.Close()
				} else {
					cancel()
				}
				select {
				case <-p.Done():
				case <-time.After(time.Second):
					t.Fatal("worker did not exit")
				}
				close(formatter.release)
				select {
				case err := <-result:
					if err == nil {
						admitted++
					} else {
						require.ErrorIs(t, err, ErrPusherClosed)
					}
				case <-time.After(time.Second):
					t.Fatal("hook did not return")
				}
				cancel()
				p.Close()
			}
			require.Zero(t, admitted, "closed worker accepted entries into an orphaned queue")
		})
	}
}
