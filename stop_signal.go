package utils

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Laisky/go-utils/v6/log"
)

var onlyOneSignalHandler = make(chan struct{})

type stopSignalOpt struct {
	closeSignals []os.Signal
	// closeFunc    func()
}

// StopSignalOptFunc options for StopSignal
type StopSignalOptFunc func(*stopSignalOpt)

// WithStopSignalCloseSignals set signals that will trigger close
func WithStopSignalCloseSignals(signals ...os.Signal) StopSignalOptFunc {
	if len(signals) == 0 {
		log.Shared.Panic("signals cannot be empty")
	}

	return func(opt *stopSignalOpt) {
		opt.closeSignals = signals
	}
}

// // WithStopSignalCloseFunc set func that will be called when signal is triggered
// func WithStopSignalCloseFunc(f func()) StopSignalOptFunc {
// 	if f == nil {
// 		log.Shared.Panic("f cannot be nil")
// 	}

// 	return func(opt *stopSignalOpt) {
// 		opt.closeFunc = f
// 	}
// }

// StopSignal registered for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
//
// Copied from https://github.com/kubernetes/sample-controller
func StopSignal(optfs ...StopSignalOptFunc) (stopCh <-chan struct{}) {
	opt := &stopSignalOpt{
		closeSignals: []os.Signal{syscall.SIGTERM, syscall.SIGINT},
		// closeFunc:    func() { os.Exit(1) },
	}
	for _, optf := range optfs {
		optf(opt)
	}

	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, opt.closeSignals...)
	go func() {
		<-c
		close(stop)
	}()

	return stop
}
