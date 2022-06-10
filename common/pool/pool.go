package pool

import "github.com/sagernet/sing/common/buf"

const (
	// io.Copy default buffer size is 32 KiB
	// but the maximum packet size of vmess/shadowsocks is about 16 KiB
	// so define a buffer of 20 KiB to reduce the memory of each TCP relay
	RelayBufferSize = 20 * 1024

	// RelayBufferSize uses 20KiB, but due to the allocator it will actually
	// request 32Kib. Most UDPs are smaller than the MTU, and the TUN's MTU
	// set to 9000, so the UDP Buffer size set to 16Kib
	UDPBufferSize = 16 * 1024
)

func Get(size int) []byte {
	return buf.Get(size)
}

func Put(buffer []byte) error {
	return buf.Put(buffer)
}
