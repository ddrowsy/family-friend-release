package servicecontrol

import "errors"

const (
	Name        = "DrowsyFriend"
	DisplayName = "Family Friend"
	Description = "Family Friend child protection service"
)

var ErrUnsupported = errors.New("Windows service management is only supported on Windows")

type State string

const (
	StateUnknown State = "unknown"
	StateStopped State = "stopped"
	StatePending State = "pending"
	StateRunning State = "running"
	StatePaused  State = "paused"
)

type StartMode string

const (
	StartModeUnknown   StartMode = "unknown"
	StartModeAutomatic StartMode = "automatic"
	StartModeManual    StartMode = "manual"
	StartModeDisabled  StartMode = "disabled"
)

type Status struct {
	Installed      bool
	State          State
	StartMode      StartMode
	ExecutablePath string
}
