package network

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
)

type muxListener struct {
	net.Listener
	tlsConfig *tls.Config
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
	return &muxListener{Listener: listener, tlsConfig: cfg}
}

func (l *muxListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		br := bufio.NewReader(conn)
		b, err := br.Peek(1)
		if err != nil {
			conn.Close()
			continue
		}
		if b[0] == 0x16 {
			return tls.Server(&peekedConn{Conn: conn, r: br}, l.tlsConfig), nil
		}
		if redirectHTTP(conn, br) {
			continue
		}
		return &peekedConn{Conn: conn, r: br}, nil
	}
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
