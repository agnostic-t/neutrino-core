package client

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall"
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
	flt          local.Filter
	directIF     string
}

func NewClient(
	p local.Proxy,
	t transport.Client,
	o obfuscation.Obfuscator,
	h handshake.HandshakeHandler,
	m nmux.Multiplexer,
	f local.Filter,
	directIF string,
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
		flt:          f,
		directIF:     directIF,
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
		if err := c.reconnectSession(); err != nil {
			return nil, err
		}
	}

	stream, err := c.session.Open()
	if err != nil {
		c.logger.Warn("Failed to open mux stream, forcing session close", "error", err)
		c.session.Close()
		c.session = nil

		if err := c.reconnectSession(); err != nil {
			return nil, err
		}
		return c.session.Open()
	}

	return stream, nil
}

func (c *Client) reconnectSession() error {
	c.logger.Info("[mux] Dialing new connection to VPN server...")
	servConn, err := c.transport.Dial()
	if err != nil {
		return err
	}

	obfsConn, err := c.obfs.WrapConnTo(servConn)
	if err != nil {
		servConn.Close()
		return err
	}

	session, err := c.muxer.Client(obfsConn)
	if err != nil {
		obfsConn.Close()
		return err
	}
	c.session = session
	return nil
}

func (c *Client) handle(req local.Request) {
	success := false
	defer func() {
		if !success {
			req.Fail(0x01)
		}
	}()

	target := req.Target()

	if c.flt != nil {
		// c.logger.Info("Calling filter on", "target", target)
		action := c.flt.Filter(target)

		if action == local.RouteBlock {
			c.logger.Info("Connection BLOCKED by filter", "target", target)
			return
		}

		if action == local.RouteDirect {
			c.logger.Info("Routing DIRECT", "target", target)
			success = c.handleDirect(req)
			return
		}
	}

	// c.logger.Info("New request", "target", req.Target())

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
	if err := c.hsher.WriteHandshake(cont_conn, target, req.Proto()); err != nil {
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

	if req.Proto() == "udp" {
		cont_conn = nmux.NewUdpFramer(cont_conn)
	}

	localConn, err := req.Success(saddr)
	if err != nil {
		c.logger.Error("Failed to write success", "error", err)
		return
	}

	if localConn == nil {
		c.logger.Warn("LConn nil, proto:", "proto", req.Proto())
		return
	}

	// fmt.Println("Starting relay, proto:", req.Proto())
	c.relay(localConn, cont_conn)
}

func (c *Client) handleDirect(req local.Request) bool {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(network, address string, rawConn syscall.RawConn) error {
			return rawConn.Control(func(fd uintptr) {
				syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, c.directIF)
			})
		},
	}

	directConn, err := dialer.Dial(req.Proto(), req.Target())
	if err != nil {
		c.logger.Error("Direct dial failed", "target", req.Target(), "error", err)
		return false
	}

	localConn, err := req.Success(directConn.LocalAddr().String())
	if err != nil {
		directConn.Close()
		c.logger.Error("Failed to write success for direct routing", "error", err)
		return false
	}
	if localConn == nil {
		directConn.Close()
		return false
	}

	c.relay(localConn, directConn)
	return true
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
