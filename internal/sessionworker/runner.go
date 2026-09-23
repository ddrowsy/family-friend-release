package sessionworker

import (
	"context"
	"fmt"
	"io"
)

type server interface {
	Serve(ctx context.Context) error
}

type acquireInstanceFunc func() (io.Closer, bool, error)
type listenFunc func() (server, error)

func run(
	ctx context.Context,
	acquireInstance acquireInstanceFunc,
	listen listenFunc,
) error {
	guard, acquired, err := acquireInstance()
	if err != nil {
		return fmt.Errorf("acquiring session worker instance: %w", err)
	}
	if !acquired {
		return nil
	}
	defer func() {
		_ = guard.Close()
	}()

	bridgeServer, err := listen()
	if err != nil {
		return fmt.Errorf("starting session bridge: %w", err)
	}
	if err := bridgeServer.Serve(ctx); err != nil {
		return fmt.Errorf("serving session bridge: %w", err)
	}
	return nil
}
