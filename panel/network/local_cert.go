package network

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync"
	"time"
)

var (
	localCert     tls.Certificate
	localCertOnce sync.Once
	localCertErr  error
)

func localhostCertificate() (tls.Certificate, error) {
	localCertOnce.Do(func() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			localCertErr = err
			return
		}
		now := time.Now()
		template := x509.Certificate{
			SerialNumber: big.NewInt(now.UnixNano()),
			Subject:      pkix.Name{CommonName: "WDTT Panel Local"},
			NotBefore:    now.Add(-time.Hour),
			NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
			DNSNames:     []string{"localhost"},
		}
		der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
		if err != nil {
			localCertErr = err
			return
		}
		localCert = tls.Certificate{
			Certificate: [][]byte{der},
			PrivateKey:  key,
		}
	})
	return localCert, localCertErr
}

func isLoopbackAddr(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
