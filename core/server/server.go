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

		if s.muxerEnabled {
			go s.handleConnection(rawConn)
		} else {
			go s.handle(rawConn)
		}
	}
}

func (s *Server) handleConnection(rawConn net.Conn) {
	defer rawConn.Close()

	obfsConn, err := s.obfs.WrapConnFrom(rawConn)
	if err != nil {
		s.logger.Error("Failed to establish obfuscated connection", "error", err)
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
		go s.handleStream(stream)
	}
}

func (s *Server) handleStream(stream net.Conn) {
	defer stream.Close()

	stream.SetDeadline(time.Now().Add(5 * time.Second))
	target, err := s.hsher.ReadHandshake(stream)
	if err != nil {
		s.logger.Error("Failed to perform handshake", "error", err)
		return
	}
	stream.SetDeadline(time.Time{})

	s.logger.Debug("Client wants to connect", "dest", target)

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		s.hsher.Failure(stream)
		return
	}
	defer targetConn.Close()

	s.hsher.Success(stream)
	s.relay(stream, targetConn)
}

func (s *Server) handle(rawConn net.Conn) {
	defer rawConn.Close()

	obfsConn, err := s.obfs.WrapConnFrom(rawConn)
	if err != nil {
		s.logger.Error("Failed to establish obfuscated connection", "error", err)
		return
	}

	obfsConn.SetDeadline(time.Now().Add(5 * time.Second))
	target, err := s.hsher.ReadHandshake(obfsConn)
	if err != nil {
		s.logger.Error("Failed to perform handshake", "error", err)
		return
	}
	obfsConn.SetDeadline(time.Time{})

	s.logger.Debug("Client wants to connect", "dest", target)

	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		s.hsher.Failure(obfsConn)
		return
	}
	defer targetConn.Close()

	s.hsher.Success(obfsConn)
	s.relay(obfsConn, targetConn)
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
