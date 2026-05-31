package handshake

import "net"

type HandshakeHandler interface {
	WriteHandshake(conn net.Conn, targ string) error
	ReadHandshake(conn net.Conn) (string, error)

	Success(conn net.Conn) bool
	Failure(conn net.Conn) bool

	ReadStatus(conn net.Conn) bool
}
