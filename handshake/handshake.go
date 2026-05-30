package handshake

import "net"

type HandshakeHandler interface {
	WriteHandshake(conn net.Conn, targ string) error
	ReadHandshake(conn net.Conn) (string, error)
}
