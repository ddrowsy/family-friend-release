//go:build windows

package main

import (
	"fmt"
	"log"
	"os"

	"fyne.io/fyne/v2/app"

	"github.com/ddrowsy/family-friend-release/internal/agent"
	"github.com/ddrowsy/family-friend-release/internal/desktopui"
)

const (
	desktopAppID          = "io.familyfriend.desktop"
	startupLaunchArgument = "--startup"
)

func main() {
	handled, err := handleStartupCommand(os.Args[1:])
	if err != nil {
		log.Fatalf("handling login startup command: %v", err)
	}
	if handled {
		return
	}

	instanceLock, acquired, err := desktopui.AcquireDesktopInstance()
	if err != nil {
		log.Fatalf("acquiring desktop UI instance: %v", err)
	}
	if !acquired {
		return
	}
	defer func() {
		if err := instanceLock.Close(); err != nil {
			log.Printf("releasing desktop UI instance: %v", err)
		}
	}()

	runUI()
}

func handleStartupCommand(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) != 1 {
		return false, fmt.Errorf("expected at most one argument, got %d", len(args))
	}
	if args[0] == startupLaunchArgument {
		return false, nil
	}

	startup := desktopui.NewLoginStartup()
	switch args[0] {
	case "--disable-startup":
		return true, startup.Disable()
	case "--enable-startup", "--startup-status":
		executablePath, err := os.Executable()
		if err != nil {
			return false, fmt.Errorf("finding desktop UI executable: %w", err)
		}
		if args[0] == "--enable-startup" {
			return true, startup.Enable(executablePath)
		}

		enabled, err := startup.Enabled(executablePath)
		if err != nil {
			return false, err
		}
		if enabled {
			fmt.Println("enabled")
			return true, nil
		}
		fmt.Println("disabled")
		return true, nil
	default:
		return false, fmt.Errorf("unsupported argument %q", args[0])
	}
}

func runUI() {
	application := app.NewWithID(desktopAppID)
	client, err := desktopui.NewHTTPAgentClient(
		"http://"+agent.DefaultUIAPIAddress,
		nil,
	)
	if err != nil {
		log.Fatalf("creating local agent client: %v", err)
	}
	shell, err := desktopui.NewShell(application, client)
	if err != nil {
		log.Fatalf("creating desktop shell: %v", err)
	}
	shell.Run()
}
