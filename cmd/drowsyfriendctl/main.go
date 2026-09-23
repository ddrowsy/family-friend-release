package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ddrowsy/family-friend-release/internal/app"
	"github.com/ddrowsy/family-friend-release/internal/config"
	"github.com/ddrowsy/family-friend-release/internal/platform/windows"
	"github.com/ddrowsy/family-friend-release/internal/servicecontrol"
	"github.com/ddrowsy/family-friend-release/internal/sessionconfig"
)

type commandKind string

const (
	commandPolicyValidate       commandKind = "policy-validate"
	commandModulesList          commandKind = "modules-list"
	commandSessionWorkerEnable  commandKind = "session-worker-enable"
	commandSessionWorkerDisable commandKind = "session-worker-disable"
	commandSessionWorkerStatus  commandKind = "session-worker-status"
	commandServiceInstall       commandKind = "service-install"
	commandServiceUninstall     commandKind = "service-uninstall"
	commandServiceStart         commandKind = "service-start"
	commandServiceStatus        commandKind = "service-status"
)

type command struct {
	kind       commandKind
	childSID   string
	workerPath string
	serviceExe string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	parsed, err := parseCommand(args)
	if err != nil {
		return err
	}

	switch parsed.kind {
	case commandPolicyValidate:
		return validatePolicy(app.DefaultPolicyPath)
	case commandModulesList:
		return listModules()
	case commandSessionWorkerEnable:
		return sessionconfig.Enable(parsed.childSID, parsed.workerPath)
	case commandSessionWorkerDisable:
		return sessionconfig.Disable()
	case commandSessionWorkerStatus:
		return showSessionWorkerStatus()
	case commandServiceInstall:
		return servicecontrol.Install(parsed.serviceExe)
	case commandServiceUninstall:
		return servicecontrol.Uninstall()
	case commandServiceStart:
		return servicecontrol.Start()
	case commandServiceStatus:
		return showServiceStatus()
	default:
		return usageError()
	}
}

func parseCommand(args []string) (command, error) {
	if len(args) < 2 {
		return command{}, usageError()
	}

	verb := args[0] + " " + args[1]
	switch verb {
	case "policy validate":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandPolicyValidate}, nil
	case "modules list":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandModulesList}, nil
	case "session-worker enable":
		if len(args) != 4 {
			return command{}, usageError()
		}
		return command{
			kind:       commandSessionWorkerEnable,
			childSID:   args[2],
			workerPath: args[3],
		}, nil
	case "session-worker disable":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandSessionWorkerDisable}, nil
	case "session-worker status":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandSessionWorkerStatus}, nil
	case "service install":
		if len(args) != 3 {
			return command{}, usageError()
		}
		return command{
			kind:       commandServiceInstall,
			serviceExe: args[2],
		}, nil
	case "service uninstall":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandServiceUninstall}, nil
	case "service start":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandServiceStart}, nil
	case "service status":
		if len(args) != 2 {
			return command{}, usageError()
		}
		return command{kind: commandServiceStatus}, nil
	default:
		return command{}, usageError()
	}
}

func validatePolicy(path string) error {
	policy, err := config.LoadPolicy(path)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}
	if err := config.ValidatePolicy(policy); err != nil {
		return fmt.Errorf("validate policy: %w", err)
	}

	fmt.Printf("policy valid: %s\n", path)
	return nil
}

func listModules() error {
	logger := log.New(os.Stderr, "", 0)
	registry := app.DefaultRegistry(windows.NewPlatform(logger), logger)
	for _, name := range registry.Names() {
		fmt.Println(name)
	}
	return nil
}

func showSessionWorkerStatus() error {
	status, err := sessionconfig.CurrentStatus()
	if err != nil {
		return err
	}
	if !status.Configured {
		fmt.Println("session worker: disabled")
		return nil
	}

	startupState := "missing"
	if status.StartupRegistered {
		startupState = "registered"
	}
	if status.StartupRegistered && !status.StartupMatches {
		startupState = "mismatched"
	}

	fmt.Println("session worker: configured")
	fmt.Printf("child SID: %s\n", status.Configuration.ChildSID)
	fmt.Printf("worker: %s\n", status.Configuration.WorkerPath)
	fmt.Printf("startup: %s\n", startupState)
	if startupState == "mismatched" {
		fmt.Printf("startup command: %s\n", status.StartupCommand)
	}
	return nil
}

func showServiceStatus() error {
	status, err := servicecontrol.CurrentStatus()
	if err != nil {
		return err
	}
	if !status.Installed {
		fmt.Println("service: not installed")
		return nil
	}

	fmt.Println("service: installed")
	fmt.Printf("name: %s\n", servicecontrol.Name)
	fmt.Printf("state: %s\n", status.State)
	fmt.Printf("startup: %s\n", status.StartMode)
	fmt.Printf("executable: %s\n", status.ExecutablePath)
	return nil
}

func usageError() error {
	return fmt.Errorf(
		"usage: drowsyfriendctl policy validate | " +
			"drowsyfriendctl modules list | " +
			"drowsyfriendctl session-worker enable <child-sid> <worker-exe> | " +
			"drowsyfriendctl session-worker disable | " +
			"drowsyfriendctl session-worker status | " +
			"drowsyfriendctl service install <agent-exe> | " +
			"drowsyfriendctl service uninstall | " +
			"drowsyfriendctl service start | " +
			"drowsyfriendctl service status",
	)
}
