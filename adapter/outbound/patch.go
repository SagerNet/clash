package outbound

import "net"

func (c *Conn) RawConn() (net.Conn, bool) {
	return c.ExtendedConn, true
}
