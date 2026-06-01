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
	"github.com/agnostic-t/neutrino-core/nmux"
	"github.com/agnostic-t/neutrino-core/obfuscation"
	"github.com/agnostic-t/neutrino-core/transport"
)

type Client struct {
	logger    *slog.Logger
	proxy     local.Proxy
	transport transport.Client
	obfs      obfuscation.Obfuscator
	hsher     handshake.HandshakeHandler

	mu      sync.Mutex
	muxer   nmux.Multiplexer
	session nmux.MultiplexerSession

	muxerEnabled bool
}

func NewClient(
	p local.Proxy,
	t transport.Client,
	o obfuscation.Obfuscator,
	h handshake.HandshakeHandler,
	m nmux.Multiplexer,
	muxerEnabled bool,
	l *slog.Logger,
) *Client {
	return &Client{
		proxy:        p,
		transport:    t,
		obfs:         o,
		logger:       l,
		hsher:        h,
		muxer:        m,
		muxerEnabled: muxerEnabled,
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

func (c *Client) getStream() (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session == nil || c.session.IsClosed() {
		c.logger.Info("[mux] Dialing new connection to VPN server...")
		servConn, err := c.transport.Dial()
		if err != nil {
			return nil, err
		}

		obfsConn, err := c.obfs.WrapConnTo(servConn)
		if err != nil {
			servConn.Close()
			return nil, err
		}

		session, err := c.muxer.Client(obfsConn)
		if err != nil {
			obfsConn.Close()
			return nil, err
		}
		c.session = session
	}

	return c.session.Open()
}

func (c *Client) handle(req local.Request) {
	success := false
	defer func() {
		if !success {
			req.Fail(0x01)
		}
	}()

	c.logger.Debug("New request", "target", req.Target())

	var servConn net.Conn
	var err error
	if c.muxerEnabled {
		servConn, err = c.getStream()
	} else {
		servConn, err = c.transport.Dial()
	}

	if err != nil {
		c.logger.Error("Failed to connect to VPN", "error", err)
		return
	}

	defer servConn.Close()

	cont_conn := servConn
	if !c.muxerEnabled {
		cont_conn, err = c.obfs.WrapConnTo(servConn)
		if err != nil {
			c.logger.Error("Failed to establish obfuscated connection", "error", err)
			return
		}
	}

	cont_conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := c.hsher.WriteHandshake(cont_conn, req.Target()); err != nil {
		c.logger.Error("Failed to read handshake", "error", err)
		return
	}

	cont_conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if !c.hsher.ReadStatus(cont_conn) {
		c.logger.Error("VPN server refused to connect to the target", "error", err)
		return
	}
	cont_conn.SetDeadline(time.Time{})

	success = true
	var saddr string
	if !c.muxerEnabled {
		saddr = cont_conn.LocalAddr().String()
	} else {
		saddr = "mux-stream"
	}

	localConn, _ := req.Success(saddr)
	c.relay(localConn, cont_conn)
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
