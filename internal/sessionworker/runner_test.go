package sessionworker

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestRunStartsServerForFirstInstance(t *testing.T) {
	guard := &closerStub{}
	bridgeServer := &serverStub{}
	listenCalls := 0

	err := run(
		context.Background(),
		func() (io.Closer, bool, error) {
			return guard, true, nil
		},
		func() (server, error) {
			listenCalls++
			return bridgeServer, nil
		},
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if listenCalls != 1 {
		t.Fatalf("listen calls = %d, want 1", listenCalls)
	}
	if bridgeServer.serveCalls != 1 {
		t.Fatalf("Serve() calls = %d, want 1", bridgeServer.serveCalls)
	}
	if !guard.closed {
		t.Fatal("instance guard was not closed")
	}
}

func TestRunSkipsDuplicateInstance(t *testing.T) {
	listenCalls := 0

	err := run(
		context.Background(),
		func() (io.Closer, bool, error) {
			return nil, false, nil
		},
		func() (server, error) {
			listenCalls++
			return &serverStub{}, nil
		},
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if listenCalls != 0 {
		t.Fatalf("listen calls = %d, want 0", listenCalls)
	}
}

func TestRunReturnsInstanceAcquisitionError(t *testing.T) {
	acquireErr := errors.New("mutex unavailable")
	listenCalls := 0

	err := run(
		context.Background(),
		func() (io.Closer, bool, error) {
			return nil, false, acquireErr
		},
		func() (server, error) {
			listenCalls++
			return &serverStub{}, nil
		},
	)
	if !errors.Is(err, acquireErr) {
		t.Fatalf("run() error = %v, want %v", err, acquireErr)
	}
	if listenCalls != 0 {
		t.Fatalf("listen calls = %d, want 0", listenCalls)
	}
}

func TestRunClosesGuardWhenListenFails(t *testing.T) {
	guard := &closerStub{}
	listenErr := errors.New("listen failed")

	err := run(
		context.Background(),
		func() (io.Closer, bool, error) {
			return guard, true, nil
		},
		func() (server, error) {
			return nil, listenErr
		},
	)
	if !errors.Is(err, listenErr) {
		t.Fatalf("run() error = %v, want %v", err, listenErr)
	}
	if !guard.closed {
		t.Fatal("instance guard was not closed")
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	guard := &closerStub{}
	started := make(chan struct{})
	bridgeServer := &serverStub{
		waitForCancellation: true,
		started:             started,
	}
	done := make(chan error, 1)

	go func() {
		done <- run(
			ctx,
			func() (io.Closer, bool, error) {
				return guard, true, nil
			},
			func() (server, error) {
				return bridgeServer, nil
			},
		)
	}()

	<-started
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !guard.closed {
		t.Fatal("instance guard was not closed")
	}
}

type closerStub struct {
	closed bool
}

func (c *closerStub) Close() error {
	c.closed = true
	return nil
}

type serverStub struct {
	serveCalls          int
	waitForCancellation bool
	started             chan struct{}
}

func (s *serverStub) Serve(ctx context.Context) error {
	s.serveCalls++
	if !s.waitForCancellation {
		return nil
	}
	close(s.started)
	<-ctx.Done()
	return nil
}
