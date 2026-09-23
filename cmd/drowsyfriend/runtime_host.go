package main

import (
	"context"
	"errors"
	"time"
)

var errRuntimeStopTimeout = errors.New("runtime stop timed out")

type runtimeStarter interface {
	Start(ctx context.Context) error
}

type managedRuntime struct {
	cancel context.CancelFunc
	done   <-chan error
}

func startManagedRuntime(runtime runtimeStarter) managedRuntime {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- runtime.Start(ctx)
	}()

	return managedRuntime{
		cancel: cancel,
		done:   done,
	}
}

func (r managedRuntime) stop(timeout time.Duration) error {
	r.cancel()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-r.done:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-timer.C:
		return errRuntimeStopTimeout
	}
}
