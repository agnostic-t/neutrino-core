package client

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/agnostic-t/neutrino-core/handshake"
	"github.com/agnostic-t/neutrino-core/local"
	"github.com/agnostic-t/neutrino-core/obfuscation"
	"github.com/agnostic-t/neutrino-core/transport"
)

type Client struct {
	logger    *slog.Logger
	proxy     local.Proxy
	transport transport.Client
	obfs      obfuscation.Obfuscator
	hsher     handshake.HandshakeHandler
}

func NewClient(p local.Proxy, t transport.Client, o obfuscation.Obfuscator, h handshake.HandshakeHandler, l *slog.Logger) *Client {
	return &Client{
		proxy:     p,
		transport: t,
		obfs:      o,
		logger:    l,
		hsher:     h,
	}
}

func (c *Client) Start(ctx context.Context) error {
	c.logger.Info("Neutrino Client is running")

	if err := c.proxy.Listen(); err != nil {
		c.logger.Error("Local proxy failed to start", "error", err)
		return err
	}

	go func() {
		<-ctx.Done()
		c.logger.Info("Shutting down proxy")
		c.proxy.Close()
	}()

	for {
		req, err := c.proxy.Accept()
		if err != nil {

			if ctx.Err() != nil {
				return nil
			}

			c.logger.Error("Proxy accept", "error", err)
			// return err
			continue
		}

		go c.handle(req)
	}
}

func (c *Client) handle(req local.Request) {
	success := false
	defer func() {
		if !success {
			req.Fail(0x01)
		}
	}()

	c.logger.Debug("New request", "target", req.Target())

	servConn, err := c.transport.Dial()
	if err != nil {
		c.logger.Error("Failed to connect to VPN", "error", err)
		return
	}
	defer servConn.Close()

	obfsConn, err := c.obfs.WrapConnTo(servConn)
	if err != nil {
		c.logger.Error("Failed to establish obfuscated connection", "error", err)
		return
	}

	obfsConn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := c.hsher.WriteHandshake(obfsConn, req.Target()); err != nil {
		c.logger.Error("Failed to read handshake", "error", err)
		return
	}
	obfsConn.SetDeadline(time.Time{})

	respBuf := make([]byte, 1)
	if _, err := io.ReadFull(obfsConn, respBuf); err != nil || respBuf[0] != 0x00 {
		c.logger.Error("VPN server refused to connect to the target", "error", err)
		return
	}

	success = true
	localConn, _ := req.Success(obfsConn.LocalAddr().String())
	c.relay(localConn, obfsConn)
}

func (c *Client) relay(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(left, right)
		left.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(right, left)
		right.Close()
	}()

	wg.Wait()
}
