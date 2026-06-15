package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pion/dtls/v3/pkg/crypto/selfsign"
)

const dtlsRenewBeforeDays = 7

func dtlsCertPaths(dir string) (certPath, keyPath string) {
	return filepath.Join(dir, "dtls-cert.pem"), filepath.Join(dir, "dtls-key.pem")
}

func dtlsCertNeedsRenewal(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return true
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true
	}
	return time.Until(cert.NotAfter) <= dtlsRenewBeforeDays*24*time.Hour
}

func generateDTLSCertPair() (tls.Certificate, error) {
	return selfsign.GenerateSelfSigned()
}

func saveDTLSCertPair(dir string, cert tls.Certificate) error {
	certPath, keyPath := dtlsCertPaths(dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return err
	}
	return os.WriteFile(keyPath, keyPEM, 0600)
}

func regenerateDTLSCert(dir string) (tls.Certificate, error) {
	cert, err := generateDTLSCertPair()
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := saveDTLSCertPair(dir, cert); err != nil {
		return tls.Certificate{}, err
	}
	certPath, _ := dtlsCertPaths(dir)
	log.Printf("[DTLS] Новый сертификат сохранён в %s", certPath)
	return cert, nil
}

func loadOrGenerateDTLSCert(dir string) (tls.Certificate, error) {
	certPath, keyPath := dtlsCertPaths(dir)
	if !dtlsCertNeedsRenewal(certPath) {
		if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
			log.Printf("[DTLS] Сертификат загружен из %s", certPath)
			return cert, nil
		}
	}
	if _, err := os.Stat(certPath); err == nil {
		log.Printf("[DTLS] Перевыпуск сертификата (истекает ≤ %d дн.)", dtlsRenewBeforeDays)
	}
	return regenerateDTLSCert(dir)
}
