package tun

import (
	"log"
	"net"
	"runtime"
	//"strconv"

	"github.com/net-byte/vtun/common/config"
	"github.com/net-byte/vtun/common/netutil"
	"github.com/net-byte/water"
	"github.com/vishvananda/netlink"
)

// CreateTun creates a tun interface
func CreateTun(config config.Config) (iFace *water.Interface) {
	c := water.Config{DeviceType: water.TUN}
	c.PlatformSpecificParams = water.PlatformSpecificParams{}
	os := runtime.GOOS
	if os == "windows" {
		c.PlatformSpecificParams.Name = "vtun"
		c.PlatformSpecificParams.Network = []string{config.CIDR, config.CIDRv6}
	}
	if config.DeviceName != "" {
		c.PlatformSpecificParams.Name = config.DeviceName
	}
	iFace, err := water.New(c)
	if err != nil {
		log.Fatalln("failed to create tun interface:", err)
		log.Fatalln("Please run with sudo or run: sudo setcap cap_net_admin=+iep <path>/vtun-linux-amd64 ", err)

	}
	log.Printf("interface created %v", iFace.Name())
	setRoute(config, iFace)
	return iFace
}

// setRoute sets the system routes
func setRoute(config config.Config, iFace *water.Interface) {
	ip, _, err := net.ParseCIDR(config.CIDR)
	if err != nil {
		log.Panicf("error cidr %v", config.CIDR)
	}
	ipv6, _, err := net.ParseCIDR(config.CIDRv6)
	if err != nil {
		log.Panicf("error ipv6 cidr %v", config.CIDRv6)
	}

	execr := netutil.ExecCmdRecorder{}
	os := runtime.GOOS
	if os == "linux" {

		netLink, err := netlink.LinkByName(iFace.Name())
		if err != nil {
			log.Printf("failed to get standard interface by name: %v", err)
			return
		}

		// Set MTU
		err = netlink.LinkSetMTU(netLink, config.MTU); 
		if err != nil {
			log.Printf("failed to set MTU: %v", err)
			return
		}

		// Add IPv4 address
		addr, err := netlink.ParseAddr(config.CIDR)
		if err != nil {
			log.Printf("failed to parse ip : %v", err)
			return
		}
		err = netlink.AddrAdd(netLink, addr)
		if err != nil {
			log.Printf("failed to set ip of tun interface: %v", err)
			return
		}

		// Add IPv6 address
		addrv6, err := netlink.ParseAddr(config.CIDRv6)
		if err != nil {
			log.Printf("failed to parse ipv6 : %v", err)
		}
		if err := netlink.AddrAdd(netLink, addrv6); err != nil {
			log.Printf("failed to set ipv6 of tun interface: %v", err)
		}

		// Bring the interface up
		err = netlink.LinkSetUp(netLink);
		if err != nil {
			log.Printf("failed to up tun interface: %v", err)
			return
		}

		if !config.ServerMode && config.GlobalMode {
			physicaliFace := netutil.GetInterface()

			physicalnetLink, err := netlink.LinkByName(physicaliFace)
			if err != nil {
				log.Printf("failed get interface: %v", err)
				return
			}

			serverAddrIP := netutil.LookupServerAddrIP(config.ServerAddr)
			if physicaliFace != "" && serverAddrIP != nil {
				if config.LocalGateway != "" {

					_, dst1, _ := net.ParseCIDR("0.0.0.0/1")
					_, dst2, _ := net.ParseCIDR("128.0.0.0/1")
					route1 := &netlink.Route{LinkIndex: netLink.Attrs().Index, Dst: dst1}
					route2 := &netlink.Route{LinkIndex: netLink.Attrs().Index, Dst: dst2}
					netlink.RouteAdd(route1)
					netlink.RouteAdd(route2)

					v4 := serverAddrIP.To4()
					
					
					if v4 != nil {
						serverAddrCIDR := &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
						gw := net.ParseIP(config.LocalGateway)
						route := &netlink.Route{LinkIndex: physicalnetLink.Attrs().Index, Dst: serverAddrCIDR, Gw: gw}
						netlink.RouteAdd(route)
					}
					

				}

				if config.LocalGatewayv6 != "" {
					// Add default IPv6 route
					defaultRoute := &netlink.Route{
						Dst:       &net.IPNet{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}, // Represents ::/0 in IPv6
						LinkIndex: netLink.Attrs().Index,
					}
					err := netlink.RouteAdd(defaultRoute)
					if err != nil {
						log.Printf("failed to add default IPv6 route: %w", err)
						return 
					}
				
					// Check if serverAddrIP is IPv6 and add specific route for it
					if serverAddrIP.To16() != nil && serverAddrIP.To4() == nil {
						route := &netlink.Route{
							Dst:       &net.IPNet{IP: serverAddrIP, Mask: net.CIDRMask(128, 128)}, // 128-bit mask for a single IPv6 address
							Gw:        net.ParseIP(config.LocalGatewayv6),
							LinkIndex: physicalnetLink.Attrs().Index,
						}
						err = netlink.RouteAdd(route)
						if err != nil {
							log.Printf("failed to add IPv6 route for server: %w", err)
							return 
						}
					}
				}
			}
		}





		
	} else if os == "darwin" {
		execr.ExecCmd("ifconfig", iFace.Name(), "inet", ip.String(), config.ServerIP, "up")
		execr.ExecCmd("ifconfig", iFace.Name(), "inet6", ipv6.String(), config.ServerIPv6, "up")
		if !config.ServerMode && config.GlobalMode {
			physicaliFace := netutil.GetInterface()
			serverAddrIP := netutil.LookupServerAddrIP(config.ServerAddr)
			if physicaliFace != "" && serverAddrIP != nil {
				if config.LocalGateway != "" {
					execr.ExecCmd("route", "add", "default", config.ServerIP)
					execr.ExecCmd("route", "change", "default", config.ServerIP)
					execr.ExecCmd("route", "add", "0.0.0.0/1", "-interface", iFace.Name())
					execr.ExecCmd("route", "add", "128.0.0.0/1", "-interface", iFace.Name())
					if serverAddrIP.To4() != nil {
						execr.ExecCmd("route", "add", serverAddrIP.To4().String(), config.LocalGateway)
					}
				}
				if config.LocalGatewayv6 != "" {
					execr.ExecCmd("route", "add", "-inet6", "default", config.ServerIPv6)
					execr.ExecCmd("route", "change", "-inet6", "default", config.ServerIPv6)
					execr.ExecCmd("route", "add", "-inet6", "::/1", "-interface", iFace.Name())
					if serverAddrIP.To16() != nil {
						execr.ExecCmd("route", "add", "-inet6", serverAddrIP.To16().String(), config.LocalGatewayv6)
					}
				}
			}
		}
	} else if os == "windows" {
		if !config.ServerMode && config.GlobalMode {
			serverAddrIP := netutil.LookupServerAddrIP(config.ServerAddr)
			if serverAddrIP != nil {
				if config.LocalGateway != "" {
					execr.ExecCmd("cmd", "/C", "route", "delete", "0.0.0.0", "mask", "0.0.0.0")
					execr.ExecCmd("cmd", "/C", "route", "add", "0.0.0.0", "mask", "0.0.0.0", config.ServerIP, "metric", "6")
					if serverAddrIP.To4() != nil {
						execr.ExecCmd("cmd", "/C", "route", "add", serverAddrIP.To4().String()+"/32", config.LocalGateway, "metric", "5")
					}
				}
				if config.LocalGatewayv6 != "" {
					execr.ExecCmd("cmd", "/C", "route", "-6", "delete", "::/0", "mask", "::/0")
					execr.ExecCmd("cmd", "/C", "route", "-6", "add", "::/0", "mask", "::/0", config.ServerIPv6, "metric", "6")
					if serverAddrIP.To16() != nil {
						execr.ExecCmd("cmd", "/C", "route", "-6", "add", serverAddrIP.To16().String()+"/128", config.LocalGatewayv6, "metric", "5")
					}
				}
			}
		}
	} else {
		log.Printf("not support os %v", os)
	}
	log.Printf("interface configured %v", iFace.Name())

	//if config.Verbose {
	//	log.Printf("set route commands:\n%s", execr.String())
	//}
}

// ResetRoute resets the system routes
func ResetRoute(config config.Config) {
	if config.ServerMode || !config.GlobalMode {
		return
	}

	os := runtime.GOOS
	execr := netutil.ExecCmdRecorder{}

	if os == "linux" {
		return
	}
	if os == "darwin" {
		if config.LocalGateway != "" {
			execr.ExecCmd("route", "add", "default", config.LocalGateway)
			execr.ExecCmd("route", "change", "default", config.LocalGateway)
		}
		if config.LocalGatewayv6 != "" {
			execr.ExecCmd("route", "add", "-inet6", "default", config.LocalGatewayv6)
			execr.ExecCmd("route", "change", "-inet6", "default", config.LocalGatewayv6)
		}
	} else if os == "windows" {
		serverAddrIP := netutil.LookupServerAddrIP(config.ServerAddr)
		if serverAddrIP != nil {
			if config.LocalGateway != "" {
				execr.ExecCmd("cmd", "/C", "route", "delete", "0.0.0.0", "mask", "0.0.0.0")
				execr.ExecCmd("cmd", "/C", "route", "add", "0.0.0.0", "mask", "0.0.0.0", config.LocalGateway, "metric", "6")
			}
			if config.LocalGatewayv6 != "" {
				execr.ExecCmd("cmd", "/C", "route", "-6", "delete", "::/0", "mask", "::/0")
				execr.ExecCmd("cmd", "/C", "route", "-6", "add", "::/0", "mask", "::/0", config.LocalGatewayv6, "metric", "6")
			}
		}
	}

	if config.Verbose {
		log.Printf("reset route commands:\n%s", execr.String())
	}
}