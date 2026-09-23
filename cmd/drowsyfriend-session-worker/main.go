package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/ddrowsy/family-friend-release/internal/sessionworker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := log.New(os.Stderr, "session-worker: ", log.LstdFlags)
	return sessionworker.Run(ctx, logger)
}
