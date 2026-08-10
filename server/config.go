package server

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

// ServerConfig — параметры одного цикла VPN-сервера.
type ServerConfig struct {
	Listen           string
	WgPort           int
	RawDirectPort    int // 0 = DTLS+3
	ConfigDir        string
	MainPass         string
	AdminID          string
	BotToken         string
	HandshakeTimeout time.Duration
	AdminAddr        string
	MaxDTLSPerDevice int
}

var (
	serverFlagsOnce sync.Once
	flagListen      *string
	flagWgPort      *int
	flagConfigDir   *string
	flagMainPass    *string
	flagAdminID     *string
	flagBotToken    *string
	flagHandshake   *time.Duration
	flagAdminAddr   *string
	flagMaxDTLSDev  *int
)

func initServerFlags() {
	serverFlagsOnce.Do(func() {
		flagListen = flag.String("listen", "0.0.0.0:56000", "DTLS адрес")
		flagWgPort = flag.Int("wg-port", defaultInternalWGPort, "WireGuard UDP порт")
		flagConfigDir = flag.String("config-dir", "/etc/wdtt", "директория конфигурации")
		flagMainPass = flag.String("password", "", "пароль владельца")
		flagAdminID = flag.String("admin", "", "Telegram Admin ID")
		flagBotToken = flag.String("bot-token", "", "Telegram Bot Token")
		flagHandshake = flag.Duration("handshake-timeout", 30*time.Second, "таймаут DTLS handshake")
		flagAdminAddr = flag.String("admin-addr", "127.0.0.1:2861", "HTTP admin (localhost): /health, POST /admin/reload")
		flagMaxDTLSDev = flag.Int("max-dtls-per-device", 0, "макс. параллельных GETCONF на device_id (0 = без лимита)")
		flag.Parse()
	})
}

func baseServerConfigFromFlags() ServerConfig {
	initServerFlags()
	return ServerConfig{
		Listen:           *flagListen,
		WgPort:           *flagWgPort,
		ConfigDir:        *flagConfigDir,
		MainPass:         *flagMainPass,
		AdminID:          *flagAdminID,
		BotToken:         *flagBotToken,
		HandshakeTimeout: *flagHandshake,
		AdminAddr:        *flagAdminAddr,
		MaxDTLSPerDevice: *flagMaxDTLSDev,
	}
}

func (c *ServerConfig) mergeFromPanelDB() {
	startup, ok, err := loadStartupFromSQLite()
	if err != nil {
		log.Printf("[CFG] inbound startup load: %v", err)
		return
	}
	if !ok {
		loadInboundSettings()
		return
	}
	if !startup.Enable {
		log.Printf("[CFG] inbound disabled in panel.db — VPN-сервер не слушает порты")
	}
	host := strings.TrimSpace(startup.ListenHost)
	if host == "" {
		host = "0.0.0.0"
	}
	if startup.DtlsPort > 0 {
		c.Listen = fmt.Sprintf("%s:%d", host, startup.DtlsPort)
	}
	if startup.WgPort > 0 {
		c.WgPort = startup.WgPort
	}
	c.RawDirectPort = startup.RawDirectPort
	if addr := strings.TrimSpace(startup.AdminAddr); addr != "" {
		c.AdminAddr = addr
	}
	if startup.HandshakeTimeoutSec >= 5 && startup.HandshakeTimeoutSec <= 600 {
		c.HandshakeTimeout = time.Duration(startup.HandshakeTimeoutSec) * time.Second
	}
	if startup.MaxDtlsPerDevice >= 0 && startup.MaxDtlsPerDevice <= 50 {
		c.MaxDTLSPerDevice = startup.MaxDtlsPerDevice
	}
	applyInboundRuntimeSettings(startup.RuntimeSettings)
	rawPort := paneldb.EffectiveRawDirectPort(startup.DtlsPort, c.RawDirectPort)
	log.Printf("[CFG] inbound startup from %s: listen=%s wg=%d raw=%d admin=%s", panelDBPath, c.Listen, c.WgPort, rawPort, c.AdminAddr)
}

func (c ServerConfig) applyGlobals() {
	dtlsHandshakeTimeout = c.HandshakeTimeout
	maxDTLSPerDevice = int32(c.MaxDTLSPerDevice)
	adminListenAddr = c.AdminAddr
}
