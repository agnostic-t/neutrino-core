package local

import "net"

type Request interface {
	Target() string

	Success(boundAddr string) (net.Conn, error)

	Fail(code int)
}

type Proxy interface {
	Listen() error
	Accept() (Request, error)
	Close() error
}
