package handshake

import "net"

type HandshakeHandler interface {
	WriteHandshake(conn net.Conn, targ string, proto string) error
	ReadHandshake(conn net.Conn) (proto string, target string, err error)

	Success(conn net.Conn) bool
	Failure(conn net.Conn) bool

	ReadStatus(conn net.Conn) bool
}
