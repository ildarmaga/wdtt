package network

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

const muxPeekTimeout = 15 * time.Second

type muxListener struct {
	net.Listener
	tlsConfig *tls.Config
	ready     chan muxConn
}

type muxConn struct {
	conn net.Conn
	err  error
}

// NewMuxListener accepts both HTTP (redirect to HTTPS) and TLS on the same port.
func NewMuxListener(listener net.Listener, tlsConfig *tls.Config) net.Listener {
	cfg := tlsConfig.Clone()
	mainCert := cfg.Certificates[0]
	local, _ := localhostCertificate()
	cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if isLoopbackAddr(hello.Conn.RemoteAddr()) && local.Certificate != nil {
			return &local, nil
		}
		return &mainCert, nil
	}
	l := &muxListener{
		Listener:  listener,
		tlsConfig: cfg,
		ready:     make(chan muxConn),
	}
	go l.acceptLoop()
	return l
}

func (l *muxListener) Accept() (net.Conn, error) {
	item, ok := <-l.ready
	if !ok {
		return nil, net.ErrClosed
	}
	return item.conn, item.err
}

func (l *muxListener) acceptLoop() {
	defer close(l.ready)
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			l.ready <- muxConn{err: err}
			return
		}
		go l.handleConn(conn)
	}
}

func (l *muxListener) handleConn(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(muxPeekTimeout))
	br := bufio.NewReader(conn)
	b, err := br.Peek(1)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return
	}
	if b[0] == 0x16 {
		l.ready <- muxConn{conn: tls.Server(&peekedConn{Conn: conn, r: br}, l.tlsConfig)}
		return
	}
	if redirectHTTP(conn, br) {
		return
	}
	l.ready <- muxConn{conn: &peekedConn{Conn: conn, r: br}}
}

type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func redirectHTTP(conn net.Conn, br *bufio.Reader) bool {
	req, err := http.ReadRequest(br)
	if err != nil {
		conn.Close()
		return true
	}
	resp := http.Response{Header: http.Header{}, StatusCode: http.StatusTemporaryRedirect}
	location := fmt.Sprintf("https://%v%v", req.Host, req.RequestURI)
	resp.Header.Set("Location", location)
	resp.Write(conn)
	conn.Close()
	return true
}
