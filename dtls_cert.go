package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"os"
	"path/filepath"

	"github.com/pion/dtls/v3/pkg/crypto/selfsign"
)

func loadOrGenerateDTLSCert(dir string) (tls.Certificate, error) {
	certPath := filepath.Join(dir, "dtls-cert.pem")
	keyPath := filepath.Join(dir, "dtls-key.pem")
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		log.Printf("[DTLS] Сертификат загружен из %s", certPath)
		return cert, nil
	}

	cert, err := selfsign.GenerateSelfSigned()
	if err != nil {
		return tls.Certificate{}, err
	}

	os.MkdirAll(dir, 0700)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}
	log.Printf("[DTLS] Новый сертификат сохранён в %s", certPath)
	return cert, nil
}
