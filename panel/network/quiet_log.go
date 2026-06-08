package network

import (
	"io"
	"os"
	"strings"
)

// QuietTLSLog drops noisy TLS handshake errors from SSH port-forwards to localhost.
type QuietTLSLog struct {
	out io.Writer
}

func NewQuietTLSLog() *QuietTLSLog {
	return &QuietTLSLog{out: os.Stderr}
}

func (l *QuietTLSLog) Write(p []byte) (int, error) {
	msg := string(p)
	if quietTLSMessage(msg) {
		return len(p), nil
	}
	_, _ = l.out.Write(p)
	return len(p), nil
}

func quietTLSMessage(msg string) bool {
	if !strings.Contains(msg, "TLS handshake error from 127.0.0.1") &&
		!strings.Contains(msg, "TLS handshake error from [::1]") {
		return false
	}
	switch {
	case strings.Contains(msg, "unknown certificate"),
		strings.Contains(msg, "certificate required"),
		strings.Contains(msg, "use of closed network connection"),
		strings.Contains(msg, "client sent an HTTP request"):
		return true
	default:
		return false
	}
}
