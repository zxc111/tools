package main

/*
139,445被isp封掉了，
监听本地139,445并转发给远端
*/

import (
	"fmt"
	"io"
	"log"
	"net"
)

const targetIp = "106.185.48.248"

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	exit := make(chan struct{})
	log.Println("000")
	go startPort("", "445", targetIp, "2001")
	go startPort("", "139", targetIp, "2000")

	for {
		select {
		case <-exit:
		}
	}
}

func startPort(localAddress, localPort, remoteAddress, remotePort string) {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%s", localAddress, localPort))
	if err != nil {
		log.Println(err)
		return
	}
	for {
		conn, err := ln.Accept()
		log.Printf("%s:%s connect", remoteAddress, remotePort)
		if err != nil {
			log.Println(err)
			return
		}
		remoteConn, err := connectRemote(remoteAddress, remotePort)
		fmt.Println(1234)
		if err != nil {
			log.Panic(err)
		}
		// go tranforData(conn, remoteConn)
		go io.Copy(conn, remoteConn)
		go io.Copy(remoteConn, conn)
	}
	return
}

func connectRemote(address, port string) (net.Conn, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", address, port))
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return conn, nil
}
