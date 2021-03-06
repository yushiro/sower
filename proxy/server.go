package proxy

import (
	"net"
	"strings"

	"github.com/golang/glog"
	"github.com/wweir/sower/parse"
	"github.com/wweir/sower/proxy/kcp"
	"github.com/wweir/sower/proxy/quic"
	"github.com/wweir/sower/proxy/tcp"
	"github.com/wweir/sower/shadow"
)

type Server interface {
	Listen(port string) (<-chan net.Conn, error)
}

func StartServer(netType, port, cipher, password string) {
	var server Server
	switch netType {
	case QUIC.String():
		server = quic.NewServer()
	case KCP.String():
		server = kcp.NewServer(password)
	case TCP.String():
		server = tcp.NewServer()
	}

	if port == "" {
		glog.Fatalln("port must set")
	}
	if !strings.Contains(port, ":") {
		port = ":" + port
	}
	connCh, err := server.Listen(port)
	if err != nil {
		glog.Fatalf("listen %v fail: %s", port, err)
	}
	glog.Infoln("Server started.")

	for {
		conn := <-connCh
		conn = shadow.Shadow(conn, cipher, password)

		go handle(conn)
	}
}

func handle(conn net.Conn) {
	conn, addr, err := parse.ParseAddr(conn)
	if err != nil {
		glog.Warningln(err)
		return
	}
	glog.V(1).Infof("new conn from %s to %s", conn.RemoteAddr(), addr)

	rc, err := net.Dial("tcp", addr)
	if err != nil {
		glog.Warningln(err)
		return
	}

	if err := rc.(*net.TCPConn).SetKeepAlive(true); err != nil {
		glog.Warningln(err)
	}

	relay(rc, conn)
}
