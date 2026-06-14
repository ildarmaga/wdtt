package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
)

type wgPeerCounters struct {
	rx int64
	tx int64
}

// wgPeerTrafficLast — последние rx/tx с wg show dump (учёт по пирам, не по DTLS-сессии).
var wgPeerTrafficLast sync.Map

func syncTrafficFromWGPeers() {
	peers, err := collectWGPeerStats()
	if err != nil {
		wgTrafficErrOnce.Do(func() {
			log.Printf("[WG] учёт трафика: %v", err)
		})
		return
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()
	for _, peer := range peers {
		if peer.pubKeyB64 == "" {
			continue
		}
		var deviceID string
		for id, dev := range db.Devices {
			if dev != nil && dev.PubKey == peer.pubKeyB64 {
				deviceID = id
				break
			}
		}
		if deviceID == "" {
			continue
		}
		pass := passwordForDeviceLocked(deviceID)
		if pass == "" {
			pass = bindOrphanDeviceToMainLocked(deviceID)
		}
		if pass == "" {
			continue
		}
		cur := wgPeerCounters{rx: peer.rx, tx: peer.tx}
		prevVal, hadPrev := wgPeerTrafficLast.Load(deviceID)
		wgPeerTrafficLast.Store(deviceID, cur)
		if !hadPrev {
			continue
		}
		prev := prevVal.(wgPeerCounters)
		if peer.rx < prev.rx || peer.tx < prev.tx {
			continue
		}
		dRx := peer.rx - prev.rx
		dTx := peer.tx - prev.tx
		if dRx > 0 {
			addTrafficLocked(pass, dRx, false)
		}
		if dTx > 0 {
			addTrafficLocked(pass, dTx, true)
		}
	}
}

type wgPeerInfo struct {
	pubKeyB64     string
	allowedIP     string
	endpoint      string
	lastHandshake int64
	rx, tx        int64
}

func deviceForWGPeerLocked(peer wgPeerInfo) (deviceID, userIP, password string, isMain bool) {
	ip := strings.TrimSuffix(peer.allowedIP, "/32")
	for id, dev := range db.Devices {
		if dev == nil {
			continue
		}
		if peer.pubKeyB64 != "" && dev.PubKey == peer.pubKeyB64 {
			pass := passwordForDeviceLocked(id)
			if pass == "" {
				continue
			}
			if dev.IP != "" {
				ip = dev.IP
			}
			return id, ip, pass, pass == db.MainPassword
		}
	}
	if ip != "" {
		id, pass, main := deviceForWGIPLocked(ip)
		return id, ip, pass, main
	}
	return "", "", "", false
}

func collectWGPeerInfos() ([]wgPeerInfo, error) {
	serverWGDevMu.RLock()
	dev := serverWGDev
	serverWGDevMu.RUnlock()
	if dev != nil {
		text, err := dev.IpcGet()
		if err == nil {
			if peers := parseWGPeerInfosIpc(text); len(peers) > 0 {
				return peers, nil
			}
		}
	}
	out, err := exec.Command("wg", "show", wgIfaceName, "dump").Output()
	if err != nil {
		return nil, fmt.Errorf("IpcGet/wg show dump: %w", err)
	}
	return parseWGPeerInfosDump(string(out)), nil
}

func parseWGPeerInfosIpc(text string) []wgPeerInfo {
	var peers []wgPeerInfo
	var cur *wgPeerInfo
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			b64, err := hexKeyToB64(v)
			if err != nil {
				continue
			}
			peers = append(peers, wgPeerInfo{pubKeyB64: b64})
			cur = &peers[len(peers)-1]
		case "endpoint":
			if cur != nil {
				cur.endpoint = strings.TrimSpace(v)
			}
		case "last_handshake_time_sec":
			if cur != nil {
				cur.lastHandshake, _ = strconv.ParseInt(v, 10, 64)
			}
		case "allowed_ip":
			if cur != nil && cur.allowedIP == "" {
				cur.allowedIP = strings.Split(strings.TrimSpace(v), "/")[0]
			}
		case "rx_bytes":
			if cur != nil {
				cur.rx, _ = strconv.ParseInt(v, 10, 64)
			}
		case "tx_bytes":
			if cur != nil {
				cur.tx, _ = strconv.ParseInt(v, 10, 64)
			}
		}
	}
	return peers
}

func parseWGPeerInfosDump(text string) []wgPeerInfo {
	var peers []wgPeerInfo
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		allowed := strings.Split(fields[3], ",")[0]
		if allowed == "off" {
			continue
		}
		handshake, _ := strconv.ParseInt(strings.TrimSpace(fields[4]), 10, 64)
		rx, errRx := strconv.ParseInt(strings.TrimSpace(fields[5]), 10, 64)
		tx, errTx := strconv.ParseInt(strings.TrimSpace(fields[6]), 10, 64)
		if errRx != nil || errTx != nil {
			continue
		}
		peers = append(peers, wgPeerInfo{
			pubKeyB64:     fields[0],
			endpoint:      fields[2],
			allowedIP:     strings.TrimSuffix(allowed, "/32"),
			lastHandshake: handshake,
			rx:            rx,
			tx:            tx,
		})
	}
	return peers
}

// syncOnlineFromWGPeers восстанавливает presence по активным WG-пирам (multi-worker relay без GETCONF).
func syncOnlineFromWGPeers() {
	peers, err := collectWGPeerInfos()
	if err != nil {
		return
	}
	now := time.Now()
	nowUnix := now.Unix()
	dbMutex.Lock()
	defer dbMutex.Unlock()
	onlineUsersMutex.Lock()
	defer onlineUsersMutex.Unlock()
	for _, peer := range peers {
		if peer.lastHandshake <= 0 || nowUnix-peer.lastHandshake > wgPeerOnlineMaxAgeSec() {
			continue
		}
		id, ip, pass, isMain := deviceForWGPeerLocked(peer)
		if id == "" {
			continue
		}
		if pass == "" {
			pass = bindOrphanDeviceToMainLocked(id)
		}
		if pass == "" {
			continue
		}
		if ip == "" {
			ip = peer.allowedIP
		}
		label := userLabel(pass, isMain)
		if info, ok := onlineUsers[id]; ok && info != nil {
			info.LastActivity = now
			if info.Sessions <= 0 {
				info.Sessions = 1
			}
			if ip != "" {
				info.IP = ip
			}
			if pass != "" {
				info.Password = pass
			}
			if label != "" {
				info.Label = label
			}
			continue
		}
		onlineUsers[id] = &onlineUserInfo{
			Label:        label,
			Password:     pass,
			IP:           ip,
			Sessions:     1,
			LastActivity: now,
		}
	}
}

func collectWGPeerStats() ([]wgPeerStat, error) {
	peers, err := collectWGPeerInfos()
	if err != nil {
		return nil, err
	}
	stats := make([]wgPeerStat, 0, len(peers))
	for _, p := range peers {
		if p.pubKeyB64 == "" {
			continue
		}
		stats = append(stats, wgPeerStat{pubKeyB64: p.pubKeyB64, rx: p.rx, tx: p.tx})
	}
	return stats, nil
}

type wgPeerStat struct {
	pubKeyB64 string
	rx, tx    int64
}

func hexKeyToB64(hexKey string) (string, error) {
	b, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(b) != 32 {
		return "", fmt.Errorf("bad wg public_key hex")
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

type wgKeys struct {
	serverPrivate, serverPublic, clientPrivate, clientPublic string
}

func b64ToHex(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	if len(b) != 32 {
		return "", fmt.Errorf("key length %d != 32", len(b))
	}
	return hex.EncodeToString(b), nil
}

func generateKeyPair() (privB64, pubB64 string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv[:]),
		base64.StdEncoding.EncodeToString(pub), nil
}

func loadOrGenerateKeys(dir string) (*wgKeys, error) {
	f := filepath.Join(dir, "wg-keys.dat")
	if data, err := os.ReadFile(f); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) >= 4 {
			keys := &wgKeys{
				serverPrivate: strings.TrimSpace(lines[0]),
				serverPublic:  strings.TrimSpace(lines[1]),
				clientPrivate: strings.TrimSpace(lines[2]),
				clientPublic:  strings.TrimSpace(lines[3]),
			}
			for _, k := range []string{keys.serverPrivate, keys.serverPublic,
				keys.clientPrivate, keys.clientPublic} {
				if _, err := b64ToHex(k); err != nil {
					goto generate
				}
			}
			log.Printf("[WG] Ключи загружены из %s", f)
			return keys, nil
		}
	}
generate:
	log.Println("[WG] Генерирую новые ключи...")
	sPriv, sPub, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	cPriv, cPub, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	keys := &wgKeys{sPriv, sPub, cPriv, cPub}
	os.MkdirAll(dir, 0700)
	os.WriteFile(f, []byte(fmt.Sprintf("%s\n%s\n%s\n%s\n",
		keys.serverPrivate, keys.serverPublic,
		keys.clientPrivate, keys.clientPublic)), 0600)
	log.Printf("[WG] Ключи сохранены в %s", f)
	return keys, nil
}

// ==================== NAT ====================

func setupFullConeNAT(wgIface string) error {
	log.Println("[NAT] ══════════════════════════════════════")

	os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644)

	setupForwardRules(wgIface)
	syncVPNLocalServices(wgIface)
	extIface := getDefaultInterface()
	log.Printf("[NAT] Внешний: %s", extIface)

	switch {
	case commandExists("iptables"):
		for i := 0; i < 5; i++ {
			exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", wgServerCIDR, "-o", extIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "MASQUERADE").Run()
		}
		exec.Command("iptables", "-t", "nat", "-I", "POSTROUTING", "1", "-s", wgServerCIDR, "-o", extIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "MASQUERADE").Run()
		natType = "MASQUERADE iptables ✅"
	case commandExists("nft"):
		setupNftNAT(extIface)
		natType = "MASQUERADE nft ✅"
	default:
		natType = "NAT не настроен: нет iptables/nft"
		log.Printf("[NAT] WARNING: %s", natType)
	}

	log.Printf("[NAT] Режим: %s", natType)
	log.Println("[NAT] ══════════════════════════════════════")
	return nil
}

var (
	vpnLocalPortsMu      sync.Mutex
	vpnLocalPortsApplied []int
)

func panelServicePorts() []int {
	panelPort, subPort := defaultPanelTCPPort, defaultSubTCPPort
	if p, s, ok, _ := loadPanelServicePortsFromSQLite(); ok {
		panelPort, subPort = p, s
	}
	ports := []int{panelPort}
	if subPort != panelPort {
		ports = append(ports, subPort)
	}
	return ports
}

func syncVPNLocalServices(wgIface string) {
	ports := panelServicePorts()
	vpnLocalPortsMu.Lock()
	defer vpnLocalPortsMu.Unlock()
	if portsEqual(vpnLocalPortsApplied, ports) && len(vpnLocalPortsApplied) > 0 {
		return
	}
	if !commandExists("iptables") {
		return
	}
	for _, port := range vpnLocalPortsApplied {
		removeVPNLocalPortRule(wgIface, port)
	}
	vpnLocalPortsApplied = append([]int(nil), ports...)
	for _, port := range ports {
		addVPNLocalPortRule(wgIface, port)
	}
	log.Printf("[NAT] Панель/подписка через VPN: tcp %v с %s", ports, wgIface)
}

func portsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func addVPNLocalPortRule(wgIface string, port int) {
	args := []string{"-i", wgIface, "-p", "tcp", "--dport", strconv.Itoa(port),
		"-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT"}
	exec.Command("iptables", append([]string{"-I", "INPUT", "1"}, args...)...).Run()
}

func removeVPNLocalPortRule(wgIface string, port int) {
	args := []string{"-i", wgIface, "-p", "tcp", "--dport", strconv.Itoa(port),
		"-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT"}
	for i := 0; i < 5; i++ {
		if exec.Command("iptables", append([]string{"-D", "INPUT"}, args...)...).Run() != nil {
			break
		}
	}
}

func setupNftNAT(extIface string) {
	exec.Command("nft", "add", "table", "ip", "wdtt").Run()
	exec.Command("nft", "add", "chain", "ip", "wdtt", "postrouting", "{ type nat hook postrouting priority 100; }").Run()
	exec.Command("nft", "add", "rule", "ip", "wdtt", "postrouting", "ip", "saddr", wgServerCIDR, "oifname", extIface, "masquerade").Run()
}

func setupForwardRules(wgIface string) {
	if commandExists("iptables") {
		for i := 0; i < 5; i++ {
			exec.Command("iptables", "-D", "FORWARD", "-i", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
			exec.Command("iptables", "-D", "FORWARD", "-o", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
		}
		// -I 1: до UFW, иначе FORWARD policy DROP режет UDP/ответы
		exec.Command("iptables", "-I", "FORWARD", "1", "-i", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
		exec.Command("iptables", "-I", "FORWARD", "1", "-o", wgIface, "-m", "comment", "--comment", "WDTT_MANAGED", "-j", "ACCEPT").Run()
		return
	}
	if commandExists("nft") {
		exec.Command("nft", "add", "table", "inet", "wdtt").Run()
		exec.Command("nft", "add", "chain", "inet", "wdtt", "forward", "{ type filter hook forward priority 0; policy accept; }").Run()
		exec.Command("nft", "add", "rule", "inet", "wdtt", "forward", "iifname", wgIface, "accept").Run()
		exec.Command("nft", "add", "rule", "inet", "wdtt", "forward", "oifname", wgIface, "accept").Run()
	}
}

// ==================== WireGuard ====================

func startUserspaceWG(keys *wgKeys, wgPort int) (*device.Device, error) {
	runCmdSilent("ip", "link", "del", wgIfaceName)
	time.Sleep(100 * time.Millisecond)

	tunDev, err := tun.CreateTUN(wgIfaceName, wgMTU)
	if err != nil {
		return nil, fmt.Errorf("CreateTUN: %w", err)
	}

	ifaceName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return nil, fmt.Errorf("TUN name: %w", err)
	}

	logger := device.NewLogger(device.LogLevelError, "[WG] ")
	bind := conn.NewDefaultBind()
	dev := device.NewDevice(tunDev, bind, logger)

	serverPrivHex, _ := b64ToHex(keys.serverPrivate)

	if err := dev.IpcSet(fmt.Sprintf(
		"private_key=%s\nlisten_port=%d\n",
		serverPrivHex, wgPort,
	)); err != nil {
		dev.Close()
		return nil, fmt.Errorf("IpcSet: %w", err)
	}

	for _, d := range db.Devices {
		pubHex, _ := b64ToHex(d.PubKey)
		if pubHex != "" {
			dev.IpcSet(fmt.Sprintf("public_key=%s\nallowed_ip=%s/32\n", pubHex, d.IP))
		}
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("device.Up: %w", err)
	}

	if err := configureInterface(ifaceName); err != nil {
		dev.Close()
		return nil, err
	}

	if err := setupFullConeNAT(ifaceName); err != nil {
		dev.Close()
		return nil, err
	}

	go func() {
		uapiFile, err := ipc.UAPIOpen(ifaceName)
		if err != nil {
			return
		}
		uapi, err := ipc.UAPIListen(ifaceName, uapiFile)
		if err != nil {
			return
		}
		defer uapi.Close()
		for {
			c, err := uapi.Accept()
			if err != nil {
				return
			}
			go dev.IpcHandle(c)
		}
	}()

	log.Printf("[WG] Запущен на порту %d", wgPort)
	return dev, nil
}

func configureInterface(ifaceName string) error {
	for _, cmd := range [][]string{
		{"ip", "addr", "add", wgServerCIDR, "dev", ifaceName},
		{"ip", "link", "set", "mtu", fmt.Sprintf("%d", wgMTU), "dev", ifaceName},
		{"ip", "link", "set", ifaceName, "up"},
	} {
		out, err := runCmd(cmd[0], cmd[1:]...)
		if err != nil && !strings.Contains(out, "File exists") {
			return fmt.Errorf("%s: %s", strings.Join(cmd, " "), out)
		}
	}
	return nil
}

func buildClientConfig(serverPublic, clientPrivate, clientIP, clientPort string) string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32
DNS = %s
MTU = %d

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0
Endpoint = 127.0.0.1:%s
PersistentKeepalive = %d`,
		clientPrivate, clientIP, clientDNS, wgMTU,
		serverPublic, clientPort, keepalive,
	)
}
