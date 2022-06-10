package inbound

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/transport/socks5"
	M "github.com/sagernet/sing/common/metadata"
)

func socksAddrToMetadata(addr M.Socksaddr) *C.Metadata {
	metadata := &C.Metadata{}
	switch addr.Family() {
	case M.AddressFamilyIPv4:
		metadata.AddrType = C.AtypIPv4
		metadata.DstIP = addr.Addr.AsSlice()
	case M.AddressFamilyIPv6:
		metadata.AddrType = C.AtypIPv6
		metadata.DstIP = addr.Addr.AsSlice()
	case M.AddressFamilyFqdn:
		metadata.AddrType = C.AtypDomainName
		metadata.Host = addr.Fqdn
	}
	metadata.DstPort = strconv.Itoa(int(addr.Port))
	return metadata
}

func parseSocksAddr(target socks5.Addr) *C.Metadata {
	metadata := &C.Metadata{
		AddrType: int(target[0]),
	}

	switch target[0] {
	case socks5.AtypDomainName:
		// trim for FQDN
		metadata.Host = strings.TrimRight(string(target[2:2+target[1]]), ".")
		metadata.DstPort = strconv.Itoa((int(target[2+target[1]]) << 8) | int(target[2+target[1]+1]))
	case socks5.AtypIPv4:
		ip := net.IP(target[1 : 1+net.IPv4len])
		metadata.DstIP = ip
		metadata.DstPort = strconv.Itoa((int(target[1+net.IPv4len]) << 8) | int(target[1+net.IPv4len+1]))
	case socks5.AtypIPv6:
		ip := net.IP(target[1 : 1+net.IPv6len])
		metadata.DstIP = ip
		metadata.DstPort = strconv.Itoa((int(target[1+net.IPv6len]) << 8) | int(target[1+net.IPv6len+1]))
	}

	return metadata
}

func parseHTTPAddr(request *http.Request) *C.Metadata {
	host := request.URL.Hostname()
	port := request.URL.Port()
	if port == "" {
		port = "80"
	}

	// trim FQDN (#737)
	host = strings.TrimRight(host, ".")

	metadata := &C.Metadata{
		NetWork:  C.TCP,
		AddrType: C.AtypDomainName,
		Host:     host,
		DstIP:    nil,
		DstPort:  port,
	}

	ip := net.ParseIP(host)
	if ip != nil {
		switch {
		case ip.To4() == nil:
			metadata.AddrType = C.AtypIPv6
		default:
			metadata.AddrType = C.AtypIPv4
		}
		metadata.DstIP = ip
	}

	return metadata
}

func parseAddr(addr string) (net.IP, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, "", err
	}

	ip := net.ParseIP(host)
	return ip, port, nil
}
