package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/agnostic-t/neutrino-core/handshake"
	"github.com/agnostic-t/neutrino-core/nmux"
	"github.com/agnostic-t/neutrino-core/obfuscation"
	"github.com/agnostic-t/neutrino-core/transport"
)

// relayBufSize: 64 KiB. The default io.Copy buffer is 32 KiB, which is too
// small for UDP framed packets (up to ~65 KiB payload) and causes
// "buffer too small for packet" errors on the UDP path.
const relayBufSize = 65535

type Server struct {
	logger *slog.Logger

	transport transport.Server
	obfs      obfuscation.Obfuscator
	hsher     handshake.HandshakeHandler

	mu           sync.Mutex
	muxer        nmux.Multiplexer
	muxerEnabled bool
}

func NewServer(
	t transport.Server,
	o obfuscation.Obfuscator,
	h handshake.HandshakeHandler,
	m nmux.Multiplexer,
	muxerEnabled bool,
	l *slog.Logger,
) *Server {
	return &Server{
		logger:       l,
		transport:    t,
		obfs:         o,
		hsher:        h,
		muxer:        m,
		muxerEnabled: muxerEnabled,
	}
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := s.transport.Listen()
	if err != nil {
		return err
	}
	defer listener.Close()

	s.logger.Info("Server started waiting for connections")

	go func() {
		<-ctx.Done()
		s.logger.Info("Shutting down server")
		listener.Close()
	}()

	acceptFails := 0
	for {
		rawConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			if errors.Is(err, net.ErrClosed) {
				return nil
			}

			// FIX: previously ANY transient accept error (e.g. EMFILE / too
			// many open files) returned from Start, silently killing the
			// whole inbound until manual restart. Now the error is logged
			// and the accept loop continues with a growing backoff, matching
			// the client behavior.
			acceptFails++
			s.logger.Error("Accept error, retrying", "error", err, "consecutive_fails", acceptFails)

			backoff := time.Duration(acceptFails) * 200 * time.Millisecond
			if backoff > time.Second {
				backoff = time.Second
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			continue
		}

		acceptFails = 0
		go s.handleRawConn(rawConn)
	}
}

func (s *Server) handleRawConn(rawConn net.Conn) {
	defer rawConn.Close()

	obfsConn, err := s.obfs.WrapConnFrom(rawConn)
	if err != nil {
		s.logger.Error("Failed to establish obfuscated connection", "error", err)
		return
	}

	if !s.muxerEnabled {
		s.processPayloadConn(obfsConn)
		return
	}

	session, err := s.muxer.Server(obfsConn)
	if err != nil {
		s.logger.Error("Failed to initialize yamux server", "error", err)
		return
	}
	defer session.Close()

	for {
		stream, err := session.Accept()
		if err != nil {
			s.logger.Debug("Session closed or stream accept error", "error", err)
			return
		}

		go s.processPayloadConn(stream)
	}
}

func (s *Server) processPayloadConn(conn net.Conn) {
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	proto, target, err := s.hsher.ReadHandshake(conn)
	if err != nil {
		s.logger.Error("Failed to perform handshake", "error", err)
		return
	}
	conn.SetDeadline(time.Time{})

	s.logger.Debug("Client wants to connect", "dest", target)

	targetConn, err := net.Dial(proto, target)
	if err != nil {
		s.hsher.Failure(conn)
		return
	}
	defer targetConn.Close()

	s.hsher.Success(conn)

	if proto == "udp" {
		conn = nmux.NewUdpFramer(conn)
	}

	s.relay(conn, targetConn)
}

func closeWriter(conn net.Conn) {
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	} else {
		conn.Close()
	}
}

func (s *Server) relay(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// FIX: dedicated 64 KiB buffer (see relayBufSize) instead of the
		// 32 KiB io.Copy default, so large framed UDP packets fit.
		io.CopyBuffer(left, right, make([]byte, relayBufSize))
		closeWriter(left)
	}()

	go func() {
		defer wg.Done()
		io.CopyBuffer(right, left, make([]byte, relayBufSize))
		closeWriter(right)
	}()

	wg.Wait()
}
