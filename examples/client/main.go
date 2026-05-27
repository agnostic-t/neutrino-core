package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/agnostic-t/neutrino-core/core/client"
	"github.com/agnostic-t/neutrino-core/local"
	"github.com/agnostic-t/neutrino-core/obfuscation"
	"github.com/agnostic-t/neutrino-core/transport"
)

func InitLocalProxy(addr string) local.Proxy {

}

func InitTCPClient(vpnServerAddr string) transport.Client {

}

func InitObfs(psk []byte) obfuscation.Obfuscator {

}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	proxy := InitLocalProxy("127.0.0.1:9000")
	defer proxy.Close()

	trans := InitTCPClient("127.0.0.1:9001")
	obfs := InitObfs([]byte("Key:IkupwyNCJrl<pRSRYrtULW&QA%TXE<"))

	client := client.NewClient(proxy, trans, obfs, logger)

	logger.Info("Starting Neutrino Client on 127.0.0.1:9000")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := client.Start(ctx); err != nil {
		logger.Error("Failed to start client", "error", err)
	}
}
