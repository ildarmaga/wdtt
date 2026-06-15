package server

import (
	"testing"
	"time"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func saveInboundGlobals(t *testing.T) func() {
	t.Helper()
	saved := struct {
		clientDNS             string
		maxGeneratedPasswords int
		dtlsHandshakeTimeout  time.Duration
		maxDTLSPerDevice      int32
		userOnlineTimeoutSec  int
		wgMTU                 int
	}{
		clientDNS:             clientDNS,
		maxGeneratedPasswords: maxGeneratedPasswords,
		dtlsHandshakeTimeout:  dtlsHandshakeTimeout,
		maxDTLSPerDevice:      maxDTLSPerDevice,
		userOnlineTimeoutSec:  userOnlineTimeoutSec,
		wgMTU:                 wgMTU,
	}
	return func() {
		clientDNS = saved.clientDNS
		maxGeneratedPasswords = saved.maxGeneratedPasswords
		dtlsHandshakeTimeout = saved.dtlsHandshakeTimeout
		maxDTLSPerDevice = saved.maxDTLSPerDevice
		userOnlineTimeoutSec = saved.userOnlineTimeoutSec
		wgMTU = saved.wgMTU
	}
}

func TestApplyInboundRuntimeSettings(t *testing.T) {
	defer saveInboundGlobals(t)()

	applyInboundRuntimeSettings(paneldb.RuntimeSettings{
		DNS:                 "8.8.8.8",
		MaxUsers:            42,
		HandshakeTimeoutSec: 45,
		MaxDtlsPerDevice:    3,
		OnlineTimeoutSec:    60,
		MTU:                 1400,
	})

	if clientDNS != "8.8.8.8" {
		t.Fatalf("clientDNS = %q", clientDNS)
	}
	if maxGeneratedPasswords != 42 {
		t.Fatalf("maxGeneratedPasswords = %d", maxGeneratedPasswords)
	}
	if dtlsHandshakeTimeout != 45*time.Second {
		t.Fatalf("dtlsHandshakeTimeout = %v", dtlsHandshakeTimeout)
	}
	if maxDTLSPerDevice != 3 {
		t.Fatalf("maxDTLSPerDevice = %d", maxDTLSPerDevice)
	}
	if userOnlineTimeoutSec != 60 {
		t.Fatalf("userOnlineTimeoutSec = %d", userOnlineTimeoutSec)
	}
	if wgMTU != 1400 {
		t.Fatalf("wgMTU = %d", wgMTU)
	}
}

func TestApplyInboundRuntimeSettingsDefaults(t *testing.T) {
	defer saveInboundGlobals(t)()

	applyInboundRuntimeSettings(paneldb.RuntimeSettings{})

	if clientDNS != defaultClientDNS {
		t.Fatalf("clientDNS = %q, want %q", clientDNS, defaultClientDNS)
	}
	if maxGeneratedPasswords != defaultMaxUsers {
		t.Fatalf("maxGeneratedPasswords = %d", maxGeneratedPasswords)
	}
	if userOnlineTimeoutSec != defaultOnlineTimeoutSec {
		t.Fatalf("userOnlineTimeoutSec = %d", userOnlineTimeoutSec)
	}
	if wgMTU != defaultWgMTU {
		t.Fatalf("wgMTU = %d", wgMTU)
	}
}
