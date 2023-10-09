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


var connList  []*net.UDPConn
var srcKeyList []string 





func stringInSlice(a string, list []string) (bool,int) {
	log.Printf("show list %v\n",list)
    for i, b := range list {
        if b == a {
            return true, i
        }
    }
    return false, len(list)
}


func closeAllConnections() {
    for _, conn := range connList {
        conn.Close()
    }
}



// Client The client struct
type RelayClient struct {
	config config.Config
	iFace  *water.Interface
	relayConn *net.UDPConn //todo: how to kill lost connections
	connCache *cache.Cache

}




// StartClient starts the udp client
func StartClientRelay(iFace *water.Interface, config config.Config) {

	localAddr, err := net.ResolveUDPAddr("udp", config.LocalAddr)
	if err != nil {
		log.Fatalln("failed to get udp socket:", err)
	}

	connRelay, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		log.Fatalln("failed to listen on udp socket:", err)
	}
	defer connRelay.Close()
	
	srcKeyList = append(srcKeyList, "127.0.0.1")

	log.Printf("vtun udp server relay started on %v", config.LocalAddr)



	log.Println("vtun udp client started")
	defer closeAllConnections()


	c := &RelayClient{config: config, iFace: iFace , relayConn: connRelay, connCache: cache.New(30*time.Minute, 10*time.Minute)}


	c.createNewConnection()
	
	go c.keepAlive()
	go c.tunToUdp()
	c.udpRelayToUdp()
}



func (c *RelayClient) createNewConnection(){
	serverAddr, err := net.ResolveUDPAddr("udp", c.config.ServerAddr)
	if err != nil {
		log.Fatalln("failed to resolve server addr:", err)
	}
	new_conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		log.Fatalln("failed to dial udp server:", err)
	}


	// Get the local address (which includes the port)
	localAddr := new_conn.LocalAddr().(*net.UDPAddr)

	log.Println("Local address:", localAddr)
	log.Println("Local port:", localAddr.Port)



	connList = append(connList, new_conn)
	conn_id := len(connList)


	
	if (conn_id==1){
		go c.udpToTun()

	} else {
		go c.udpToRelay(conn_id)
	}
	
	log.Printf("created socket with vtun server conn_id %d\n",conn_id)
	log.Printf("conn list -->> %v\n",connList)
	
}


// udpToTun sends packets from udp to tun
func (c *RelayClient) udpToTun() {
	packet := make([]byte, c.config.BufferSize)
	for {
		n, err := connList[0].Read(packet)
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
		c.iFace.Write(b)
		counter.IncrReadBytes(n)
	}
}



// udpToRelay sends packets from udp to udp
func (c *RelayClient) udpToRelay(conn_id int) {
	packet := make([]byte, c.config.BufferSize)
	for {
		n, err := connList[conn_id-1].Read(packet)
		if err != nil {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}

		b := packet[:n]
		b_ori := b



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

		if key := netutil.GetDstKey(b); key != "" {
			if v, ok := c.connCache.Get(key); ok {

				_, err := c.relayConn.WriteToUDP(b_ori, v.(*net.UDPAddr))
				if err != nil {
					c.connCache.Delete(key)
					continue
				}
				//counter.IncrWrittenBytes(n)
			}
		}


		log.Printf("send pkg back udpRelay")
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
		_, err = connList[0].Write(b)
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

		_, err := connList[0].Write(pingIpPacket)
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

		n, cliAddr, err := c.relayConn.ReadFromUDP(packet)
		if err != nil || n == 0 {
			netutil.PrintErr(err, c.config.Verbose)
			continue
		}

		b := packet[:n]
		b_ori := b



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


		//HERE WE NEED TO CHANGE THE SRC IP BECAUSE WHEN REACH THE SERVER THE RESPONSE OF THE REQUEST IT DOEST KNOW KNWO TO REPLY


		if srcKey := netutil.GetSrcKey(b); srcKey != "" {

			c.connCache.Set(srcKey, cliAddr, 24*time.Hour)


			ret, index := stringInSlice(srcKey,srcKeyList)
			if ( !ret ){
				log.Printf("create a new connection for srcKey %s",srcKey)
				c.createNewConnection()
				srcKeyList = append(srcKeyList, srcKey)
			}
	
			if dstKey := netutil.GetDstKey(b); dstKey != "" {
				log.Printf("relay pkg to server dst %s conn %d",dstKey, index)
			}
	
	
			_, err = connList[index].Write(b_ori)
			if err != nil {
				netutil.PrintErr(err, c.config.Verbose)
				continue
			}



			c.relayConn.Write(b)

			continue
		}


		log.Println("pkg ignored")


		
	}
}