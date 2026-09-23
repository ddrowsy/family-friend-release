//go:build windows

package main

import (
	"log"
	"time"

	"golang.org/x/sys/windows/svc"
)

type windowsServiceHandler struct {
	runtime     runtimeStarter
	logger      *log.Logger
	stopTimeout time.Duration
}

func (h *windowsServiceHandler) Execute(
	_ []string,
	requests <-chan svc.ChangeRequest,
	statuses chan<- svc.Status,
) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}

	runtime := startManagedRuntime(h.runtime)
	statuses <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	for {
		select {
		case err := <-runtime.done:
			if err != nil {
				h.logError("runtime stopped unexpectedly", err)
				return false, 1
			}
			return false, 0
		case request, ok := <-requests:
			if !ok {
				return h.stop(runtime, statuses)
			}

			switch request.Cmd {
			case svc.Interrogate:
				statuses <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				return h.stop(runtime, statuses)
			}
		}
	}
}

func (h *windowsServiceHandler) stop(
	runtime managedRuntime,
	statuses chan<- svc.Status,
) (bool, uint32) {
	timeout := h.stopTimeout
	if timeout <= 0 {
		timeout = serviceStopTimeout
	}

	statuses <- svc.Status{
		State:    svc.StopPending,
		WaitHint: uint32(timeout.Milliseconds()),
	}
	if err := runtime.stop(timeout); err != nil {
		h.logError("stopping runtime", err)
		return false, 1
	}

	// svc.Run reports the final Stopped state after Execute returns.
	return false, 0
}

func (h *windowsServiceHandler) logError(message string, err error) {
	if h.logger == nil {
		return
	}
	h.logger.Printf("%s: %v", message, err)
}
