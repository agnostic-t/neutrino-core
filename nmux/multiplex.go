package nmux

import "net"

type MultiplexerSession interface {
	Open() (net.Conn, error)
	Close() error
	Accept() (net.Conn, error)
	IsClosed() bool
}

type Multiplexer interface {
	Client(conn net.Conn) (MultiplexerSession, error)
	Server(conn net.Conn) (MultiplexerSession, error)
}
