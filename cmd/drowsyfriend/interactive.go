package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func runInteractive(runtime runtimeStarter) error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	return runtime.Start(ctx)
}
