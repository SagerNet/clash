package net

import (
	"context"
	"net"

	"github.com/sagernet/sing/common/bufio"
)

// Relay copies between left and right bidirectionally.
func Relay(leftConn, rightConn net.Conn) {
	bufio.CopyConn(context.Background(), leftConn, rightConn)
}
