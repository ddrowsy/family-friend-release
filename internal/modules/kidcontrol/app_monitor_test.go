package kidcontrol

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

type fakeProcessProvider struct {
	mu    sync.Mutex
	count int
}

func (f *fakeProcessProvider) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	f.mu.Lock()
	f.count++
	f.mu.Unlock()
	return []platform.ProcessInfo{{PID: 1, Name: "msedge.exe", Path: `C:\mock\msedge.exe`}}, nil
}

func (f *fakeProcessProvider) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.count
}

func TestAppMonitorScanReadsProcessesFromProvider(t *testing.T) {
	provider := &fakeProcessProvider{}
	monitor := NewAppMonitor(provider, time.Second, []string{"msedge.exe"}, log.New(io.Discard, "", 0))

	observations, err := monitor.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	if provider.Count() == 0 {
		t.Fatal("ListProcesses was not called")
	}
	if len(observations) != 1 {
		t.Fatalf("len(observations) = %d, want 1", len(observations))
	}
}

func TestAppMonitorLoopDetectsBlockedAppAndStopsWhenContextCancelled(t *testing.T) {
	provider := &fakeProcessProvider{}
	detected := make(chan struct{}, 1)
	monitor := NewAppMonitor(provider, 10*time.Millisecond, []string{"msedge.exe"}, log.New(io.Discard, "", 0))
	monitor.SetHandler(func(ctx context.Context, observations []AppObservation) error {
		if len(observations) > 0 && observations[0].Process.Name == "msedge.exe" {
			select {
			case detected <- struct{}{}:
			default:
			}
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	select {
	case <-detected:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for app observation")
	}

	cancel()
	if err := monitor.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	countAfterStop := provider.Count()
	time.Sleep(25 * time.Millisecond)
	if got := provider.Count(); got != countAfterStop {
		t.Fatalf("process scans after stop = %d, want %d", got, countAfterStop)
	}
}
