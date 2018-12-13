package proxy

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/golang/glog"
	"github.com/lucas-clemente/quic-go"
)

func StartClient(server string) {
	connCh := listenLocal([]string{":80", ":443"})
	reDialCh := make(chan net.Conn, 10)
	var conn net.Conn
	var count int

	for {
		sess, err := quic.DialAddr(server, &tls.Config{InsecureSkipVerify: true}, dialConf)
		if err != nil {
			if sess, err = quic.DialAddr(server, &tls.Config{InsecureSkipVerify: true}, dialConf); err != nil {
				glog.Errorf("connect to remote(%s) fail:%s\n", server, err)
				time.Sleep(2 * time.Second)
				continue
			}
		}
		glog.Infof("new session from (%s) to (%s)", sess.LocalAddr(), sess.RemoteAddr())

		count = 0
		for { // session rotate logic
			select {
			case conn = <-connCh:
			case conn = <-reDialCh:
			}
			count++

			// sync action to reuse sigle sess
			if !openStream(conn, sess, count, reDialCh) {
				sess.Close()
				break
			}
		}
	}
}

func openStream(conn net.Conn, sess quic.Session, count int, reDialCh chan<- net.Conn) bool {
	glog.V(2).Infoln("new request from", conn.RemoteAddr())

	okCh := make(chan struct{})
	go func() {
		stream, err := sess.OpenStream()
		if err != nil {
			glog.Warningf("start stream to (%s) fail:%s\n", sess.RemoteAddr(), err)
			reDialCh <- conn
			close(okCh)
			return
		}
		defer stream.Close()

		glog.V(2).Infof("START stream\t%d", count)
		defer glog.V(2).Infof("CLOSE stream\t%d", count)

		select {
		case okCh <- struct{}{}:
		default:
			close(okCh)
			return
		}
		close(okCh)

		if err := conn.(*net.TCPConn).SetKeepAlive(true); err != nil {
			glog.Warningln(err)
		}
		relay(sess, &streamConn{stream, sess}, conn)
		conn.Close()
	}()

	select {
	case _, ok := <-okCh: // false means close on error
		return ok
	case <-time.After(500 * time.Millisecond):
		return false
	}
}

func listenLocal(ports []string) <-chan net.Conn {
	connCh := make(chan net.Conn, 10)
	for i := range ports {
		go func(port string) {
			ln, err := net.Listen("tcp", port)
			if err != nil {
				glog.Fatalln(err)
			}

			for {
				conn, err := ln.Accept()
				if err != nil {
					glog.Errorln("accept", port, "fail:", err)
					continue
				}

				connCh <- conn
			}
		}(ports[i])
	}

	glog.Infoln("listening ports:", ports)
	return connCh
}
