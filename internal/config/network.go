package config

import (
	"net"
	"os"
	"sync"
)

var (
	localIPOnce sync.Once
	localIP     string
)

// LocalIP is this machine's LAN address: the interface the default route
// leaves through (found with a UDP "dial", which sends nothing), else the first
// non-loopback IPv4 address that is up, else 127.0.0.1. It fills in the
// connection details on broker sheets when ME_PUB_IP / ME_INT_IP are not set.
func LocalIP() string {
	localIPOnce.Do(func() {
		if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
			if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() && !a.IP.IsUnspecified() {
				localIP = a.IP.String()
			}
			c.Close()
		}
		if localIP == "" {
			localIP = firstInterfaceIP()
		}
	})
	return localIP
}

func firstInterfaceIP() string {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
				return n.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// Hostname is this machine's name ("localhost" if it cannot be read).
func Hostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "localhost"
}
