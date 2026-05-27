package transport

import "net"

/*
- VPN Client
  - Purpose is connection to the VPN server and providing transperent for read()/write() (net.Conn) socket
*/
type Client interface {

	// Dials connection with VPN server, no IP, port or domain is required in sake of modularity.
	Dial() (net.Conn, error)
}

/*
- VPN Server
  - Purpose is listeninig and accepting incoming connections to the VPN-server
*/
type Server interface {

	// Returns net.Listener, that
	Listen() (net.Listener, error)
}
