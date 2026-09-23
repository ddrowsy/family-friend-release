package main

import "testing"

func TestParseRuntimeOptionsDefaultsToProductionData(t *testing.T) {
	options, err := parseRuntimeOptions(nil)
	if err != nil {
		t.Fatalf("parseRuntimeOptions returned error: %v", err)
	}
	if options.MockData {
		t.Fatal("MockData = true, want false")
	}
}

func TestParseRuntimeOptionsEnablesMockData(t *testing.T) {
	options, err := parseRuntimeOptions([]string{"--mock-data"})
	if err != nil {
		t.Fatalf("parseRuntimeOptions returned error: %v", err)
	}
	if !options.MockData {
		t.Fatal("MockData = false, want true")
	}
}

func TestParseRuntimeOptionsRejectsUnknownArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown"},
		{"unexpected"},
	} {
		if _, err := parseRuntimeOptions(args); err == nil {
			t.Fatalf("parseRuntimeOptions(%q) returned nil error", args)
		}
	}
}
