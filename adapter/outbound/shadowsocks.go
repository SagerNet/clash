package outbound

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Dreamacro/clash/common/structure"
	"github.com/Dreamacro/clash/component/dialer"
	C "github.com/Dreamacro/clash/constant"
	obfs "github.com/Dreamacro/clash/transport/simple-obfs"
	"github.com/Dreamacro/clash/transport/socks5"
	v2rayObfs "github.com/Dreamacro/clash/transport/v2ray-plugin"
	"github.com/sagernet/sing/common"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/random"
	"github.com/sagernet/sing/protocol/shadowsocks"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/protocol/shadowsocks/shadowstream"
)

type ShadowSocks struct {
	*Base
	method shadowsocks.Method

	// obfs
	obfsMode    string
	obfsOption  *simpleObfsOption
	v2rayOption *v2rayObfs.Option
}

type ShadowSocksOption struct {
	BasicOption
	Name       string         `proxy:"name"`
	Server     string         `proxy:"server"`
	Port       int            `proxy:"port"`
	Password   string         `proxy:"password,omitempty"`
	Key        string         `proxy:"key,omitempty"`
	Cipher     string         `proxy:"cipher"`
	UDP        bool           `proxy:"udp,omitempty"`
	Plugin     string         `proxy:"plugin,omitempty"`
	PluginOpts map[string]any `proxy:"plugin-opts,omitempty"`
}

type simpleObfsOption struct {
	Mode string `obfs:"mode,omitempty"`
	Host string `obfs:"host,omitempty"`
}

type v2rayObfsOption struct {
	Mode           string            `obfs:"mode"`
	Host           string            `obfs:"host,omitempty"`
	Path           string            `obfs:"path,omitempty"`
	TLS            bool              `obfs:"tls,omitempty"`
	Headers        map[string]string `obfs:"headers,omitempty"`
	SkipCertVerify bool              `obfs:"skip-cert-verify,omitempty"`
	Mux            bool              `obfs:"mux,omitempty"`
}

// StreamConn implements C.ProxyAdapter
func (ss *ShadowSocks) StreamConn(c net.Conn, metadata *C.Metadata) (net.Conn, error) {
	switch ss.obfsMode {
	case "tls":
		c = obfs.NewTLSObfs(c, ss.obfsOption.Host)
	case "http":
		_, port, _ := net.SplitHostPort(ss.addr)
		c = obfs.NewHTTPObfs(c, ss.obfsOption.Host, port)
	case "websocket":
		var err error
		c, err = v2rayObfs.NewV2rayObfs(c, ss.v2rayOption)
		if err != nil {
			return nil, fmt.Errorf("%s connect error: %w", ss.addr, err)
		}
	}
	return ss.method.DialConn(c, M.ParseSocksaddr(metadata.RemoteAddress()))
}

// DialContext implements C.ProxyAdapter
func (ss *ShadowSocks) DialContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (_ C.Conn, err error) {
	c, err := dialer.DialContext(ctx, "tcp", ss.addr, ss.Base.DialOptions(opts...)...)
	if err != nil {
		return nil, fmt.Errorf("%s connect error: %w", ss.addr, err)
	}
	tcpKeepAlive(c)

	defer safeConnClose(c, err)

	c, err = ss.StreamConn(c, metadata)
	return NewConn(c, ss), err
}

// ListenPacketContext implements C.ProxyAdapter
func (ss *ShadowSocks) ListenPacketContext(ctx context.Context, metadata *C.Metadata, opts ...dialer.Option) (C.PacketConn, error) {
	pc, err := dialer.ListenPacket(ctx, "udp", "", ss.Base.DialOptions(opts...)...)
	if err != nil {
		return nil, err
	}

	addr, err := resolveUDPAddr("udp", ss.addr)
	if err != nil {
		pc.Close()
		return nil, err
	}
	pc = ss.method.DialPacketConn(&N.BindPacketConn{PacketConn: pc, Addr: addr})
	return newPacketConn(pc, ss), nil
}

func NewShadowSocks(option ShadowSocksOption) (*ShadowSocks, error) {
	addr := net.JoinHostPort(option.Server, strconv.Itoa(option.Port))
	cipher := option.Cipher
	var method shadowsocks.Method
	var err error
	if cipher == "dummy" || cipher == "none" {
		method = shadowsocks.NewNone()
	} else if common.Contains(shadowstream.List, cipher) {
		var key []byte
		if option.Key != "" {
			key, err = base64.StdEncoding.DecodeString(option.Key)
			if err == nil {
				goto returnErr
			}
		}
		method, err = shadowstream.New(cipher, key, []byte(option.Password), random.Default)
		if err != nil {
			goto returnErr
		}
	} else if common.Contains(shadowaead.List, cipher) {
		var key []byte
		if option.Key != "" {
			key, err = base64.StdEncoding.DecodeString(option.Key)
			if err == nil {
				goto returnErr
			}
		}
		method, err = shadowaead.New(cipher, key, []byte(option.Password), random.Default)
		if err != nil {
			goto returnErr
		}
	} else if common.Contains(shadowaead_2022.List, cipher) {
		var pskList [][shadowaead_2022.KeySaltSize]byte
		if option.Key != "" {
			for _, pskStr := range strings.Split(option.Key, ":") {
				var pskB []byte
				pskB, err = base64.StdEncoding.DecodeString(pskStr)
				if err != nil {
					goto returnErr
				}
				if len(pskB) != shadowaead_2022.KeySaltSize {
					err = fmt.Errorf("bad psk")
					goto returnErr
				}
				var psk [shadowaead_2022.KeySaltSize]byte
				copy(psk[:], pskB)
				pskList = append(pskList, psk)
			}
		}
		method, err = shadowaead_2022.New(cipher, pskList, random.Default)
		if err != nil {
			goto returnErr
		}
	} else {
		err = fmt.Errorf("unsupported cipher %s", cipher)
	}
returnErr:
	if err != nil {
		return nil, fmt.Errorf("ss %s initialize error: %w", addr, err)
	}

	var v2rayOption *v2rayObfs.Option
	var obfsOption *simpleObfsOption
	obfsMode := ""

	decoder := structure.NewDecoder(structure.Option{TagName: "obfs", WeaklyTypedInput: true})
	if option.Plugin == "obfs" {
		opts := simpleObfsOption{Host: "bing.com"}
		if err := decoder.Decode(option.PluginOpts, &opts); err != nil {
			return nil, fmt.Errorf("ss %s initialize obfs error: %w", addr, err)
		}

		if opts.Mode != "tls" && opts.Mode != "http" {
			return nil, fmt.Errorf("ss %s obfs mode error: %s", addr, opts.Mode)
		}
		obfsMode = opts.Mode
		obfsOption = &opts
	} else if option.Plugin == "v2ray-plugin" {
		opts := v2rayObfsOption{Host: "bing.com", Mux: true}
		if err := decoder.Decode(option.PluginOpts, &opts); err != nil {
			return nil, fmt.Errorf("ss %s initialize v2ray-plugin error: %w", addr, err)
		}

		if opts.Mode != "websocket" {
			return nil, fmt.Errorf("ss %s obfs mode error: %s", addr, opts.Mode)
		}
		obfsMode = opts.Mode
		v2rayOption = &v2rayObfs.Option{
			Host:    opts.Host,
			Path:    opts.Path,
			Headers: opts.Headers,
			Mux:     opts.Mux,
		}

		if opts.TLS {
			v2rayOption.TLS = true
			v2rayOption.SkipCertVerify = opts.SkipCertVerify
		}
	}

	return &ShadowSocks{
		Base: &Base{
			name:  option.Name,
			addr:  addr,
			tp:    C.Shadowsocks,
			udp:   option.UDP,
			iface: option.Interface,
			rmark: option.RoutingMark,
		},
		method: method,

		obfsMode:    obfsMode,
		v2rayOption: v2rayOption,
		obfsOption:  obfsOption,
	}, nil
}

type ssPacketConn struct {
	net.PacketConn
	rAddr net.Addr
}

func (spc *ssPacketConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	packet, err := socks5.EncodeUDPPacket(socks5.ParseAddrToSocksAddr(addr), b)
	if err != nil {
		return
	}
	return spc.PacketConn.WriteTo(packet[3:], spc.rAddr)
}

func (spc *ssPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, _, e := spc.PacketConn.ReadFrom(b)
	if e != nil {
		return 0, nil, e
	}

	addr := socks5.SplitAddr(b[:n])
	if addr == nil {
		return 0, nil, errors.New("parse addr error")
	}

	udpAddr := addr.UDPAddr()
	if udpAddr == nil {
		return 0, nil, errors.New("parse addr error")
	}

	copy(b, b[len(addr):])
	return n - len(addr), udpAddr, e
}
