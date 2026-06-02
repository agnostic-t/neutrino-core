package server

import (
	"context"
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

	for {
		rawConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			s.logger.Error("Accept error", "error", err)
			return err
		}

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
		io.Copy(left, right)
		closeWriter(right)
	}()

	go func() {
		defer wg.Done()
		io.Copy(right, left)
		closeWriter(left)
	}()

	wg.Wait()
}
