package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/agnostic-t/neutrino-core/handshake"
	"github.com/agnostic-t/neutrino-core/obfuscation"
	"github.com/agnostic-t/neutrino-core/transport"
)

type Server struct {
	logger *slog.Logger

	transport transport.Server
	obfs      obfuscation.Obfuscator
	hsher     handshake.HandshakeHandler
}

func NewServer(t transport.Server, o obfuscation.Obfuscator, h handshake.HandshakeHandler, l *slog.Logger) *Server {
	return &Server{
		logger:    l,
		transport: t,
		obfs:      o,
		hsher:     h,
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

		go s.handle(rawConn)
	}
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
