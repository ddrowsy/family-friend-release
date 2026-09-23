//go:build !windows

package windows

import (
	"context"
	"io"
	"log"
	"testing"
)

func TestPlatformListProcessesReturnsProcesses(t *testing.T) {
	platform := NewPlatform(log.New(io.Discard, "", 0))

	processes, err := platform.ListProcesses(context.Background())
	if err != nil {
		t.Fatalf("ListProcesses returned error: %v", err)
	}

	names := map[string]bool{}
	for _, process := range processes {
		names[process.Name] = true
	}

	for _, want := range []string{"explorer.exe", "msedge.exe", "notepad.exe"} {
		if !names[want] {
			t.Fatalf("process %q missing from sample processes: %#v", want, processes)
		}
	}
}
