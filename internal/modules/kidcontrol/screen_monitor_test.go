package kidcontrol

import (
	"context"
	"io"
	"log"
	"testing"

	"github.com/ddrowsy/family-friend-release/internal/platform"
)

type fakeScreenProvider struct {
	called bool
}

func (f *fakeScreenProvider) CaptureScreen(ctx context.Context) (platform.ScreenFrame, error) {
	f.called = true
	return platform.ScreenFrame{
		Width:  800,
		Height: 600,
		Source: "test",
		Data:   []byte("frame"),
	}, nil
}

func TestScreenMonitorReadsFrameFromProvider(t *testing.T) {
	provider := &fakeScreenProvider{}
	monitor := NewScreenMonitor(provider, 3, log.New(io.Discard, "", 0))

	if err := monitor.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if !provider.called {
		t.Fatal("CaptureScreen was not called")
	}
}
