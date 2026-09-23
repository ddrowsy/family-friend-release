//go:build !windows

package servicecontrol

func Install(string) error {
	return ErrUnsupported
}

func Uninstall() error {
	return ErrUnsupported
}

func Start() error {
	return ErrUnsupported
}

func CurrentStatus() (Status, error) {
	return Status{}, ErrUnsupported
}
