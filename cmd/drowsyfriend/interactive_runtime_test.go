package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestNewInteractiveRuntimeMockDataWarnsClearly(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)

	runtime, err := newInteractiveRuntime(logger, runtimeOptions{MockData: true})
	if err != nil {
		t.Fatalf("newInteractiveRuntime returned error: %v", err)
	}
	if runtime.PolicyPath != "" {
		t.Fatalf("PolicyPath = %q, want embedded mock policy", runtime.PolicyPath)
	}
	if !strings.Contains(output.String(), "WARNING: mock data mode enabled") {
		t.Fatalf("log output = %q, want mock-data warning", output.String())
	}
}

func TestNewInteractiveRuntimeProductionUsesDefaultPolicyPath(t *testing.T) {
	runtime, err := newInteractiveRuntime(log.Default(), runtimeOptions{})
	if err != nil {
		t.Fatalf("newInteractiveRuntime returned error: %v", err)
	}
	if runtime.PolicyPath != "configs/default.policy.json" {
		t.Fatalf("PolicyPath = %q, want default production policy path", runtime.PolicyPath)
	}
}
