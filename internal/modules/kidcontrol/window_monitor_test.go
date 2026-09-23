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

type fakeWindowProvider struct {
	mu    sync.Mutex
	count int
}

func (f *fakeWindowProvider) ActiveWindow(ctx context.Context) (platform.WindowInfo, error) {
	f.mu.Lock()
	f.count++
	f.mu.Unlock()
	return platform.WindowInfo{
		ID:          "window-1",
		Title:       "Mock Window",
		ProcessID:   10,
		ProcessName: "msedge.exe",
	}, nil
}

func (f *fakeWindowProvider) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.count
}

func TestWindowMonitorReadsActiveWindowFromProvider(t *testing.T) {
	provider := &fakeWindowProvider{}
	monitor := NewWindowMonitor(provider, time.Second, log.New(io.Discard, "", 0))

	observation, err := monitor.Observe(context.Background())
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}

	if provider.Count() == 0 {
		t.Fatal("ActiveWindow was not called")
	}
	if observation.Window.ProcessName != "msedge.exe" {
		t.Fatalf("ProcessName = %q, want msedge.exe", observation.Window.ProcessName)
	}
}

func TestWindowMonitorStopsWhenContextCancelled(t *testing.T) {
	provider := &fakeWindowProvider{}
	detected := make(chan struct{}, 1)
	monitor := NewWindowMonitor(provider, 10*time.Millisecond, log.New(io.Discard, "", 0))
	monitor.SetHandler(func(ctx context.Context, observation WindowObservation) error {
		if observation.Window.ProcessName == "msedge.exe" {
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
		t.Fatal("timed out waiting for window observation")
	}

	cancel()
	if err := monitor.Stop(context.Background()); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	countAfterStop := provider.Count()
	time.Sleep(25 * time.Millisecond)
	if got := provider.Count(); got != countAfterStop {
		t.Fatalf("window scans after stop = %d, want %d", got, countAfterStop)
	}
}
