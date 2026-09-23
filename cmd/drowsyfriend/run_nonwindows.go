//go:build !windows

package main

import "log"

func run(logger *log.Logger, options runtimeOptions) error {
	runtime, err := newInteractiveRuntime(logger, options)
	if err != nil {
		return err
	}
	return runInteractive(runtime)
}
