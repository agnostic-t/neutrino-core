package obfuscation

import "net"

/*
- Obfuscator interface
  - obj.WrapConn: returns wrapped connection, any read() and write() shall be obfuscated and deobfuscated
*/
type Obfuscator interface {
	WrapConnTo(conn net.Conn) (net.Conn, error)
	WrapConnFrom(conn net.Conn) (net.Conn, error)
}
