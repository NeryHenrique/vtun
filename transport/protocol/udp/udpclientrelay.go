package udp

import (
	"log"
	"net"
	"time"

	"github.com/golang/snappy"
	"github.com/net-byte/vtun/common/cipher"
	"github.com/net-byte/vtun/common/config"
	"github.com/net-byte/vtun/common/counter"
	"github.com/net-byte/vtun/common/netutil"
	"github.com/net-byte/water"
	"github.com/patrickmn/go-cache"
)

// Client The client struct
type RelayClient struct {
	config config.Config
	iFace  *water.Interface
	conn   *net.UDPConn

	relayConn *net.UDPConn
	connCache *cache.Cache
}



// StartClient starts the udp client
func StartClientRelay(iFace *water.Interface, config config.Config) {
	serverAddr, err := net.ResolveUDPAddr("udp", config.ServerAddr)
	if err != nil {
		log.Fatalln("failed to resolve server addr:", err)
	}
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		log.Fatalln("failed to dial udp server:", err)
	}
	defer conn.Close()
	log.Println("vtun udp client started")



	log.Printf("vtun udp relay server started on %v", config.LocalAddr)
	localAddr, err := net.ResolveUDPAddr("udp", config.LocalAddr)
	if err != nil {
		log.Fatalln("failed to get udp socket:", err)
	}

	connRelay, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		log.Fatalln("failed to listen on udp socket:", err)
	}
	defer connRelay.Close()
	
	c := &RelayClient{config: config, iFace: iFace, conn: conn, relayConn: connRelay, connCache: cache.New(30*time.Minute, 10*time.Minute)}
	go c.udpToTunOrUdpRelay()
	go c.keepAlive()
	go c.tunToUdp()
	c.udpRelayToUdp()
}

// udpToTun sends packets from udp to tun
func (c *RelayClient) udpToTunOrUdpRelay() {
	packet := make([]byte, c.config.BufferSize)
	for {
		n, err := c.conn.Read(packet)
		if err != nil {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}

		b := packet[:n]
		if c.config.Compress {
			b, err = snappy.Decode(nil, b)
			if err != nil {
				netutil.PrintErr(err, c.config.Verbose)
				continue
			}
		}
		if c.config.Obfs {
			b = cipher.XOR(b)
		}

		if dstKey := netutil.GetDstKey(b); dstKey != "" {
			log.Printf("pkg dst %s", dstKey)
		}
		//todo: check if the current client is the destiny or is to relay
		c.iFace.Write(b)
		counter.IncrReadBytes(n)
	}
}

// tunToUdp sends packets from tun to udp
func (c *RelayClient) tunToUdp() {
	packet := make([]byte, c.config.BufferSize)
	for {
		n, err := c.iFace.Read(packet)
		if err != nil {
			netutil.PrintErr(err, c.config.Verbose)
			break
		}
		b := packet[:n]
		if c.config.Obfs {
			b = cipher.XOR(b)
		}
		if c.config.Compress {
			b = snappy.Encode(nil, b)
		}
		_, err = c.conn.Write(b)
		if err != nil {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}
		counter.IncrWrittenBytes(n)
	}
}

func (c *RelayClient) keepAlive() {
	srcIp, _, err := net.ParseCIDR(c.config.CIDR)
	if err != nil {
		netutil.PrintErr(err, c.config.Verbose)
		return
	}

	// dst ip(pingIpPacket[12:16]): 0.0.0.0, src ip(pingIpPacket[16:20]): 0.0.0.0
	pingIpPacket := []byte{0x45, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x40, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00}
	copy(pingIpPacket[12:16], srcIp[12:16]) // modify ping packet src ip to client CIDR ip

	if c.config.Obfs {
		pingIpPacket = cipher.XOR(pingIpPacket)
	}
	if c.config.Compress {
		pingIpPacket = snappy.Encode(nil, pingIpPacket)
	}

	for {
		time.Sleep(time.Second * 10)

		_, err := c.conn.Write(pingIpPacket)
		if err != nil {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}
	}
}



// udpRelayToUdp sends packets from udpRelay to udpClient
func (c *RelayClient) udpRelayToUdp() {
	packet := make([]byte, c.config.BufferSize)
	for {
		n, err := c.relayConn.Read(packet)
		if err != nil || n == 0 {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}

		b := packet[:n]
		_, err = c.conn.Write(b)
		if err != nil {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}


		if dstKey := netutil.GetDstKey(b); dstKey != "" {
			log.Printf("relay pkg to server dst %s",dstKey)
		}

		
	}
}