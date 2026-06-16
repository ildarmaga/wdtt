package panel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const dtlsRenewBeforeDays = 7

func panelRegenerateDTLSCert() error {
	certPath := wdttDtlsCertFile
	keyPath := wdttDtlsKeyFile
	dir := filepath.Dir(certPath)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "self-signed cert"},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 1, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	raw, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return err
	}
	return nil
}

func panelDTLSCertNeedsRenewal() bool {
	data, err := os.ReadFile(wdttDtlsCertFile)
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

func renewDTLSCert(restart bool, cfg *PanelConfig) (map[string]interface{}, error) {
	if _, err := os.Stat(wdttDtlsCertFile); err != nil && os.IsNotExist(err) {
		return nil, fmt.Errorf("DTLS сертификат не найден")
	}
	if err := panelRegenerateDTLSCert(); err != nil {
		return nil, fmt.Errorf("не удалось перевыпустить DTLS: %w", err)
	}
	if restart {
		if isUnifiedDeployment() {
			inbound, _ := loadWdttInbound()
			if err := wdttServerRestart(inbound.AdminAddr); err != nil {
				return nil, fmt.Errorf("сертификат обновлён, но restart VPN не удался: %w", err)
			}
			if inbound.Enable {
				applyWdttMtuRules("up")
			}
		} else if err := serviceRestart(wdttServiceUnit); err != nil {
			return nil, fmt.Errorf("сертификат обновлён, но restart wdtt не удался: %w", err)
		} else if !waitServiceActive(wdttServiceUnit, 20*time.Second) {
			return nil, fmt.Errorf("сертификат обновлён, но wdtt-server не поднялся")
		}
	}
	certs, _ := listCertificates(cfg)
	msg := "DTLS сертификат обновлён"
	if restart {
		msg += ", wdtt-server перезапущен"
	}
	return map[string]interface{}{
		"message": msg,
		"certs":   certs,
	}, nil
}

func renewDTLSCertIfNeeded(restart bool, cfg *PanelConfig) error {
	if !panelDTLSCertNeedsRenewal() {
		return nil
	}
	_, err := renewDTLSCert(restart, cfg)
	return err
}
