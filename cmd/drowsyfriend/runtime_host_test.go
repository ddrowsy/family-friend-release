package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type runtimeStarterFunc func(ctx context.Context) error

func (f runtimeStarterFunc) Start(ctx context.Context) error {
	return f(ctx)
}

func TestManagedRuntimeStopCancelsAndWaits(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})

	runtime := startManagedRuntime(
		runtimeStarterFunc(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(finished)
			return ctx.Err()
		}),
	)

	<-started
	if err := runtime.stop(time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}

	select {
	case <-finished:
	default:
		t.Fatal("runtime did not finish before stop returned")
	}
}

func TestManagedRuntimeStopReturnsRuntimeError(t *testing.T) {
	started := make(chan struct{})
	wantErr := errors.New("stop failed")

	runtime := startManagedRuntime(
		runtimeStarterFunc(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return wantErr
		}),
	)

	<-started
	if err := runtime.stop(time.Second); !errors.Is(err, wantErr) {
		t.Fatalf("stop error = %v, want %v", err, wantErr)
	}
}

func TestManagedRuntimeStopTimesOut(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	runtime := startManagedRuntime(
		runtimeStarterFunc(func(context.Context) error {
			close(started)
			<-release
			return nil
		}),
	)

	<-started
	if err := runtime.stop(20 * time.Millisecond); !errors.Is(
		err,
		errRuntimeStopTimeout,
	) {
		t.Fatalf("stop error = %v, want %v", err, errRuntimeStopTimeout)
	}

	close(release)
	select {
	case <-runtime.done:
	case <-time.After(time.Second):
		t.Fatal("runtime did not exit after test release")
	}
}
