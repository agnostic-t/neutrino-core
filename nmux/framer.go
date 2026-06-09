package nmux

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

type UdpFramer struct {
	net.Conn
}

func NewUdpFramer(conn net.Conn) *UdpFramer {
	return &UdpFramer{Conn: conn}
}

func (f *UdpFramer) Read(b []byte) (int, error) {
	var length uint16
	if err := binary.Read(f.Conn, binary.BigEndian, &length); err != nil {
		return 0, err
	}

	if int(length) > len(b) {
		return 0, fmt.Errorf("buffer too small for packet: %d", length)
	}

	return io.ReadFull(f.Conn, b[:length])
}

func (f *UdpFramer) Write(b []byte) (int, error) {
	if err := binary.Write(f.Conn, binary.BigEndian, uint16(len(b))); err != nil {
		return 0, err
	}

	return f.Conn.Write(b)
}

func (f *UdpFramer) CloseWrite() error {
	if cw, ok := f.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}

	return f.Conn.Close()
}
