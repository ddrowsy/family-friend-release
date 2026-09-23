//go:build windows

package sessionbridge

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPipeSecurityDescriptorRestrictsServerAndClient(t *testing.T) {
	serverSID := "S-1-5-21-1-2-3-1001"
	clientSID := "S-1-5-18"

	got, err := pipeSecurityDescriptor(serverSID, clientSID)
	if err != nil {
		t.Fatalf("pipeSecurityDescriptor() error = %v", err)
	}

	want := fmt.Sprintf(
		"D:P(A;;GA;;;%s)(A;;0x%08x;;;%s)",
		serverSID,
		uint32(pipeClientAccess),
		clientSID,
	)
	if got != want {
		t.Fatalf("security descriptor = %q, want %q", got, want)
	}
	if strings.Contains(got, "WD") {
		t.Fatalf("security descriptor grants Everyone access: %q", got)
	}
}

func TestPipeSecurityDescriptorRejectsInvalidSID(t *testing.T) {
	_, err := pipeSecurityDescriptor(
		"S-1-5-21-1-2-3-1001)(A;;GA;;;WD",
		"S-1-5-18",
	)
	if err == nil {
		t.Fatal("pipeSecurityDescriptor() error = nil")
	}
}

func TestPipeClientAccessCannotCreatePipeInstances(t *testing.T) {
	if pipeClientAccess&windows.FILE_APPEND_DATA != 0 {
		t.Fatalf("pipe client access includes FILE_CREATE_PIPE_INSTANCE bit: %#x", pipeClientAccess)
	}
}
