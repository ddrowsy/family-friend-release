//go:build windows

package main

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestWindowsServiceHandlerStopLifecycle(t *testing.T) {
	started := make(chan struct{})
	runtime := runtimeStarterFunc(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil
	})
	handler := &windowsServiceHandler{
		runtime:     runtime,
		logger:      log.New(io.Discard, "", 0),
		stopTimeout: time.Second,
	}
	requests := make(chan svc.ChangeRequest, 2)
	statuses := make(chan svc.Status, 4)
	exitCode := make(chan uint32, 1)

	go func() {
		_, code := handler.Execute(
			[]string{},
			requests,
			statuses,
		)
		exitCode <- code
	}()

	assertServiceState(t, statuses, svc.StartPending)
	running := nextServiceStatus(t, statuses)
	if running.State != svc.Running {
		t.Fatalf("state = %v, want %v", running.State, svc.Running)
	}
	wantAccepts := svc.AcceptStop | svc.AcceptShutdown
	if running.Accepts != wantAccepts {
		t.Fatalf("accepts = %v, want %v", running.Accepts, wantAccepts)
	}
	<-started

	requests <- svc.ChangeRequest{
		Cmd:           svc.Interrogate,
		CurrentStatus: running,
	}
	assertServiceState(t, statuses, svc.Running)

	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	assertServiceState(t, statuses, svc.StopPending)

	select {
	case code := <-exitCode:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(time.Second):
		t.Fatal("service handler did not stop")
	}
}

func TestWindowsServiceHandlerStopTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runtime := runtimeStarterFunc(func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	handler := &windowsServiceHandler{
		runtime:     runtime,
		logger:      log.New(io.Discard, "", 0),
		stopTimeout: 20 * time.Millisecond,
	}
	requests := make(chan svc.ChangeRequest, 1)
	statuses := make(chan svc.Status, 3)
	exitCode := make(chan uint32, 1)

	go func() {
		_, code := handler.Execute(
			[]string{},
			requests,
			statuses,
		)
		exitCode <- code
	}()

	assertServiceState(t, statuses, svc.StartPending)
	assertServiceState(t, statuses, svc.Running)
	<-started

	requests <- svc.ChangeRequest{Cmd: svc.Shutdown}
	assertServiceState(t, statuses, svc.StopPending)

	select {
	case code := <-exitCode:
		if code == 0 {
			t.Fatal("timeout returned zero exit code")
		}
	case <-time.After(time.Second):
		t.Fatal("service handler did not return after timeout")
	}

	close(release)
}

func assertServiceState(
	t *testing.T,
	statuses <-chan svc.Status,
	want svc.State,
) {
	t.Helper()

	status := nextServiceStatus(t, statuses)
	if status.State != want {
		t.Fatalf("state = %v, want %v", status.State, want)
	}
}

func nextServiceStatus(
	t *testing.T,
	statuses <-chan svc.Status,
) svc.Status {
	t.Helper()

	select {
	case status := <-statuses:
		return status
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service status")
		return svc.Status{}
	}
}
