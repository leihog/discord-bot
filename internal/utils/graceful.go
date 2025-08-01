package utils

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// SetupGracefulShutdown sets up signal handling and returns a context that will be cancelled on shutdown
func SetupGracefulShutdown() (context.Context, context.CancelFunc) {
	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Set up signal handling
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler goroutine
	go func() {
		<-shutdownChan
		cancel()
	}()

	return ctx, cancel
}
