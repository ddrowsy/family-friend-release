package main

import "testing"

func TestParseCommandSessionWorkerEnable(t *testing.T) {
	parsed, err := parseCommand([]string{
		"session-worker",
		"enable",
		"S-1-5-21-1001",
		`C:\Family Friend\worker.exe`,
	})
	if err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	if parsed.kind != commandSessionWorkerEnable {
		t.Fatalf("kind = %q, want %q", parsed.kind, commandSessionWorkerEnable)
	}
	if parsed.childSID != "S-1-5-21-1001" {
		t.Fatalf("child SID = %q", parsed.childSID)
	}
	if parsed.workerPath != `C:\Family Friend\worker.exe` {
		t.Fatalf("worker path = %q", parsed.workerPath)
	}
}

func TestParseCommandSessionWorkerLifecycleCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want commandKind
	}{
		{
			name: "disable",
			args: []string{"session-worker", "disable"},
			want: commandSessionWorkerDisable,
		},
		{
			name: "status",
			args: []string{"session-worker", "status"},
			want: commandSessionWorkerStatus,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseCommand(test.args)
			if err != nil {
				t.Fatalf("parseCommand() error = %v", err)
			}
			if parsed.kind != test.want {
				t.Fatalf("kind = %q, want %q", parsed.kind, test.want)
			}
		})
	}
}

func TestParseCommandServiceInstall(t *testing.T) {
	parsed, err := parseCommand([]string{
		"service",
		"install",
		`C:\Family Friend\drowsyfriend.exe`,
	})
	if err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	if parsed.kind != commandServiceInstall {
		t.Fatalf("kind = %q, want %q", parsed.kind, commandServiceInstall)
	}
	if parsed.serviceExe != `C:\Family Friend\drowsyfriend.exe` {
		t.Fatalf("service executable = %q", parsed.serviceExe)
	}
}

func TestParseCommandServiceLifecycleCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want commandKind
	}{
		{
			name: "uninstall",
			args: []string{"service", "uninstall"},
			want: commandServiceUninstall,
		},
		{
			name: "start",
			args: []string{"service", "start"},
			want: commandServiceStart,
		},
		{
			name: "status",
			args: []string{"service", "status"},
			want: commandServiceStatus,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseCommand(test.args)
			if err != nil {
				t.Fatalf("parseCommand() error = %v", err)
			}
			if parsed.kind != test.want {
				t.Fatalf("kind = %q, want %q", parsed.kind, test.want)
			}
		})
	}
}

func TestParseCommandRejectsInvalidLifecycleArguments(t *testing.T) {
	tests := [][]string{
		{"session-worker"},
		{"session-worker", "enable", "S-1-5-21-1001"},
		{"session-worker", "disable", "extra"},
		{"session-worker", "unknown"},
		{"service"},
		{"service", "install"},
		{"service", "status", "extra"},
		{"service", "unknown"},
	}

	for _, args := range tests {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("parseCommand(%q) error = nil", args)
		}
	}
}
