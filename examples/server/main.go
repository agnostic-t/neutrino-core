package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/agnostic-t/neutrino-core/core/server"
	"github.com/agnostic-t/neutrino-core/obfuscation"
	"github.com/agnostic-t/neutrino-core/transport"
)

func InitTCPTransport(bindAddr string) transport.Server {

}

func InitObfs() obfuscation.Obfuscator {

}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	trans := InitTCPTransport("0.0.0.0:9001")
	obfs := InitObfs()

	server := server.NewServer(trans, obfs, logger)

	logger.Info("Server is starting at 0.0.0.0:9001")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		logger.Error("Failed to start server", "error", err)
	}
}
