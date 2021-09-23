package udp

import (
	"log"
	"net"

	"github.com/net-byte/vtun/common/cipher"
	"github.com/net-byte/vtun/common/config"
	"github.com/net-byte/vtun/tun"
	"github.com/songgao/water/waterutil"
)

// StartClient starts udp client
func StartClient(config config.Config) {
	iface := tun.CreateTun(config)
	serverAddr, err := net.ResolveUDPAddr("udp", config.ServerAddr)
	if err != nil {
		log.Fatalln("failed to resolve server addr:", err)
	}
	localAddr, err := net.ResolveUDPAddr("udp", config.LocalAddr)
	if err != nil {
		log.Fatalln("failed to get udp socket:", err)
	}
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		log.Fatalln("failed to listen on udp socket:", err)
	}
	defer conn.Close()
	log.Printf("vtun udp client started on %v", config.LocalAddr)
	// server -> client
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil || n == 0 {
				continue
			}
			var b []byte
			if config.Obfuscate {
				b = cipher.XOR(buf[:n])
			} else {
				b = buf[:n]
			}
			if !waterutil.IsIPv4(b) {
				continue
			}
			iface.Write(b)
		}
	}()
	// client -> server
	packet := make([]byte, 1500)
	for {
		n, err := iface.Read(packet)
		if err != nil || n == 0 {
			continue
		}
		if !waterutil.IsIPv4(packet) {
			continue
		}
		var b []byte
		if config.Obfuscate {
			b = cipher.XOR(packet[:n])
		} else {
			b = packet[:n]
		}
		conn.WriteToUDP(b, serverAddr)
	}
}
