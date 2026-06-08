package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type processState string

const (
	stateRunning processState = "running"
	stateStop    processState = "stop"
	stateError   processState = "error"
)

type serverStatus struct {
	CPU         float64 `json:"cpu"`
	CPUCores    int     `json:"cpuCores"`
	LogicalPro  int     `json:"logicalPro"`
	CPUSpeedMhz float64 `json:"cpuSpeedMhz"`
	Mem         struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"mem"`
	Swap struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"swap"`
	Disk struct {
		Current uint64 `json:"current"`
		Total   uint64 `json:"total"`
	} `json:"disk"`
	Xray struct {
		State    processState `json:"state"`
		ErrorMsg string       `json:"errorMsg"`
		Version  string       `json:"version"`
		Uptime   uint64       `json:"uptime"`
	} `json:"xray"`
	Wdtt struct {
		State       processState `json:"state"`
		ErrorMsg    string       `json:"errorMsg"`
		ActiveUsers int          `json:"activeUsers"`
		Sessions    int          `json:"sessions"`
		Iface       string       `json:"iface"`
		MainPass    string       `json:"mainPassword"`
		DownGB      string       `json:"downGB"`
		UpGB        string       `json:"upGB"`
		Uptime      uint64       `json:"uptime"`
	} `json:"wdtt"`
	Uptime   uint64    `json:"uptime"`
	Loads    []float64 `json:"loads"`
	TCPCount int       `json:"tcpCount"`
	UDPCount int       `json:"udpCount"`
	NetIO    struct {
		Up   uint64 `json:"up"`
		Down uint64 `json:"down"`
	} `json:"netIO"`
	NetTraffic struct {
		Sent uint64 `json:"sent"`
		Recv uint64 `json:"recv"`
	} `json:"netTraffic"`
	PublicIP struct {
		IPv4 string `json:"ipv4"`
		IPv6 string `json:"ipv6"`
	} `json:"publicIP"`
	AppStats struct {
		Panel  serviceUsage `json:"panel"`
		Xray   serviceUsage `json:"xray"`
		Wdtt   serviceUsage `json:"wdtt"`
		Uptime uint64       `json:"uptime"`
	} `json:"appStats"`
}

type serviceUsage struct {
	Mem     uint64 `json:"mem"`
	Threads uint32 `json:"threads"`
}

var (
	statusMu       sync.Mutex
	cachedStatus   *serverStatus
	lastCPUIdle    uint64
	lastCPUTotal   uint64
	hasCPUSample   bool
	lastVpnRx      uint64
	lastVpnTx      uint64
	hasVpnSample   bool
	lastVpnSampleT time.Time
	cpuHistory     []map[string]interface{}
	panelStart     = time.Now()
	cachedIPv4     string
	cachedIPv6     string
	cachedCPUCores int
	cachedCPUMhz   float64
)

func startStatusCollector() {
	refreshCachedStatus()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C {
			ensureXrayFollowsWdtt()
			refreshCachedStatus()
		}
	}()
	go func() {
		refreshPublicIPs()
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			refreshPublicIPs()
		}
	}()
}

func getCachedServerStatus() *serverStatus {
	statusMu.Lock()
	defer statusMu.Unlock()
	if cachedStatus == nil {
		return &serverStatus{}
	}
	cp := *cachedStatus
	return &cp
}

func refreshCachedStatus() {
	s := collectServerStatus()
	statusMu.Lock()
	cachedStatus = s
	statusMu.Unlock()
}

func collectServerStatus() *serverStatus {
	s := &serverStatus{}
	s.CPU = readCPUPercent()
	if cachedCPUCores == 0 {
		cachedCPUCores, _ = strconv.Atoi(strings.TrimSpace(firstLine("nproc")))
		if cachedCPUCores == 0 {
			cachedCPUCores = 1
		}
	}
	if cachedCPUMhz == 0 {
		cachedCPUMhz = readCPUSpeedMhz()
	}
	s.CPUCores = cachedCPUCores
	s.LogicalPro = cachedCPUCores
	s.CPUSpeedMhz = cachedCPUMhz

	memCur, memTot := readMem()
	s.Mem.Current, s.Mem.Total = memCur, memTot
	swapCur, swapTot := readSwap()
	s.Swap.Current, s.Swap.Total = swapCur, swapTot
	diskCur, diskTot := readDisk()
	s.Disk.Current, s.Disk.Total = diskCur, diskTot
	s.Loads = readLoads()
	s.Uptime = readOSUptime()
	s.TCPCount, s.UDPCount = readConnCounts()
	s.NetIO.Up, s.NetIO.Down = vpnTrafficSpeed()
	s.NetTraffic.Sent, s.NetTraffic.Recv = vpnTrafficTotals()
	s.PublicIP.IPv4 = cachedIPv4
	s.PublicIP.IPv6 = cachedIPv6
	if s.PublicIP.IPv4 == "" {
		s.PublicIP.IPv4 = "N/A"
	}
	if s.PublicIP.IPv6 == "" {
		s.PublicIP.IPv6 = "N/A"
	}

	s.AppStats.Uptime = uint64(time.Since(panelStart).Seconds())
	s.AppStats.Panel = readServiceUsage(panelServiceUnit)
	s.AppStats.Xray = readServiceUsage(xrayServiceUnit)
	s.AppStats.Wdtt = readServiceUsage(wdttServiceUnit)

	fillXrayStatus(s)
	fillWdttStatus(s)

	appendCPUHistory(s.CPU)
	return s
}

func fillXrayStatus(s *serverStatus) {
	s.Xray.Version = xrayVersionShort()
	if serviceActive(xrayServiceUnit) {
		s.Xray.State = stateRunning
		if t, err := serviceUptime(xrayServiceUnit); err == nil {
			s.Xray.Uptime = t
		}
		return
	}
	if serviceUnitFailed(xrayServiceUnit) {
		s.Xray.State = stateError
		s.Xray.ErrorMsg = getXrayRestartResult()
		return
	}
	s.Xray.State = stateStop
}

func fillWdttStatus(s *serverStatus) {
	stats := loadServerStats()
	db, _ := loadPasswords()
	if serviceActive(wdttServiceUnit) {
		s.Wdtt.State = stateRunning
	} else {
		s.Wdtt.State = stateStop
		out, _ := runCmd("journalctl", "-u", wdttServiceUnit, "-n", "5", "--no-pager", "-o", "cat")
		s.Wdtt.ErrorMsg = strings.TrimSpace(out)
	}
	if stats != nil {
		s.Wdtt.ActiveUsers = stats.ActiveUsers
		s.Wdtt.Sessions = stats.Sessions
		s.Wdtt.DownGB = stats.DownGB
		s.Wdtt.UpGB = stats.UpGB
	}
	if db != nil {
		s.Wdtt.MainPass = db.MainPassword
	}
	s.Wdtt.Iface = getWdttIface()
	if t, err := serviceUptime(wdttServiceUnit); err == nil {
		s.Wdtt.Uptime = t
	}
}

func readServiceUsage(unit string) serviceUsage {
	pid, err := serviceMainPID(unit)
	if err != nil || pid <= 0 {
		return serviceUsage{}
	}
	return readProcessUsage(pid)
}

func serviceMainPID(unit string) (int, error) {
	out, err := runCmd("systemctl", "show", unit, "--property=MainPID", "--value")
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func readProcessUsage(pid int) serviceUsage {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return serviceUsage{}
	}
	var u serviceUsage
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				kb, _ := strconv.ParseUint(f[1], 10, 64)
				u.Mem = kb * 1024
			}
		} else if strings.HasPrefix(line, "Threads:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				n, _ := strconv.ParseUint(f[1], 10, 32)
				u.Threads = uint32(n)
			}
		}
	}
	return u
}

func serviceUptime(unit string) (uint64, error) {
	out, err := runCmd("systemctl", "show", unit, "--property=ActiveEnterTimestamp", "--value")
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", out)
	if err != nil {
		return 0, err
	}
	secs := time.Since(t).Seconds()
	if secs < 0 {
		return 0, nil
	}
	return uint64(secs), nil
}

func readCPUPercent() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line := strings.Split(string(data), "\n")[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
		if i == 4 {
			idle = v
		}
	}
	statusMu.Lock()
	defer statusMu.Unlock()
	if !hasCPUSample {
		lastCPUIdle, lastCPUTotal, hasCPUSample = idle, total, true
		return 0
	}
	idleDelta := float64(idle - lastCPUIdle)
	totalDelta := float64(total - lastCPUTotal)
	lastCPUIdle, lastCPUTotal = idle, total
	if totalDelta <= 0 {
		return 0
	}
	return (1.0 - idleDelta/totalDelta) * 100
}

func readCPUSpeedMhz() float64 {
	data, _ := os.ReadFile("/proc/cpuinfo")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "cpu MHz") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				v, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				return v
			}
		}
	}
	return 0
}

func readMem() (cur, total uint64) {
	out, _ := runCmd("free", "-b")
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Mem:") {
			f := strings.Fields(line)
			if len(f) >= 3 {
				total, _ = strconv.ParseUint(f[1], 10, 64)
				cur, _ = strconv.ParseUint(f[2], 10, 64)
			}
		}
	}
	return
}

func readSwap() (cur, total uint64) {
	out, _ := runCmd("free", "-b")
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Swap:") {
			f := strings.Fields(line)
			if len(f) >= 3 {
				total, _ = strconv.ParseUint(f[1], 10, 64)
				cur, _ = strconv.ParseUint(f[2], 10, 64)
			}
		}
	}
	return
}

func readDisk() (cur, total uint64) {
	out, _ := runCmd("df", "-B1", "/")
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return
	}
	f := strings.Fields(lines[1])
	if len(f) >= 4 {
		total, _ = strconv.ParseUint(f[1], 10, 64)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		cur = used
	}
	return
}

func readLoads() []float64 {
	data, _ := os.ReadFile("/proc/loadavg")
	f := strings.Fields(string(data))
	out := []float64{0, 0, 0}
	for i := 0; i < 3 && i < len(f); i++ {
		out[i], _ = strconv.ParseFloat(f[i], 64)
	}
	return out
}

func readOSUptime() uint64 {
	data, _ := os.ReadFile("/proc/uptime")
	f := strings.Fields(string(data))
	if len(f) > 0 {
		v, _ := strconv.ParseFloat(f[0], 64)
		return uint64(v)
	}
	return 0
}

func readConnCounts() (tcp, udp int) {
	data, _ := os.ReadFile("/proc/net/tcp")
	tcp = max(0, len(strings.Split(string(data), "\n"))-2)
	data, _ = os.ReadFile("/proc/net/udp")
	udp = max(0, len(strings.Split(string(data), "\n"))-2)
	return
}

const vpnTrafficIface = "wdtt0"

func vpnTrafficTotals() (sent, recv uint64) {
	rx, tx := readIfaceBytes(vpnTrafficIface)
	return tx, rx
}

func vpnTrafficSpeed() (up, down uint64) {
	rx, tx := readIfaceBytes(vpnTrafficIface)
	now := time.Now()
	statusMu.Lock()
	defer statusMu.Unlock()
	if hasVpnSample {
		dt := now.Sub(lastVpnSampleT).Seconds()
		if dt > 0 {
			if tx >= lastVpnTx {
				up = uint64(float64(tx-lastVpnTx) / dt)
			}
			if rx >= lastVpnRx {
				down = uint64(float64(rx-lastVpnRx) / dt)
			}
		}
	}
	lastVpnRx, lastVpnTx = rx, tx
	lastVpnSampleT = now
	hasVpnSample = true
	return
}

func readIfaceBytes(iface string) (rx, tx uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != iface {
			continue
		}
		flds := strings.Fields(parts[1])
		if len(flds) >= 9 {
			rx, _ = strconv.ParseUint(flds[0], 10, 64)
			tx, _ = strconv.ParseUint(flds[8], 10, 64)
		}
		return
	}
	return
}

func refreshPublicIPs() {
	if v4, _ := runCmd("curl", "-4", "-s", "--max-time", "2", "ifconfig.me"); v4 != "" {
		cachedIPv4 = strings.TrimSpace(v4)
	} else if out, _ := runCmd("hostname", "-I"); out != "" {
		cachedIPv4 = strings.Fields(out)[0]
	}
	if v6, _ := runCmd("curl", "-6", "-s", "--max-time", "2", "ifconfig.me"); v6 != "" {
		cachedIPv6 = strings.TrimSpace(v6)
	}
}

func appendCPUHistory(cpu float64) {
	statusMu.Lock()
	defer statusMu.Unlock()
	cpuHistory = append(cpuHistory, map[string]interface{}{
		"t":   time.Now().Unix(),
		"cpu": cpu,
	})
	if len(cpuHistory) > 3600 {
		cpuHistory = cpuHistory[len(cpuHistory)-3600:]
	}
}

func aggregateCPUHistory(bucketSec, maxPoints int) []map[string]interface{} {
	statusMu.Lock()
	hist := append([]map[string]interface{}{}, cpuHistory...)
	statusMu.Unlock()
	if len(hist) == 0 {
		return []map[string]interface{}{}
	}
	var out []map[string]interface{}
	var acc []float64
	curBucket := hist[0]["t"].(int64) / int64(bucketSec) * int64(bucketSec)
	flush := func(ts int64) {
		if len(acc) == 0 {
			return
		}
		sum := 0.0
		for _, v := range acc {
			sum += v
		}
		out = append(out, map[string]interface{}{"t": ts, "cpu": sum / float64(len(acc))})
		acc = acc[:0]
	}
	for _, p := range hist {
		ts := p["t"].(int64)
		b := ts / int64(bucketSec) * int64(bucketSec)
		if b != curBucket {
			flush(curBucket)
			curBucket = b
		}
		acc = append(acc, p["cpu"].(float64))
	}
	flush(curBucket)
	if len(out) > maxPoints {
		out = out[len(out)-maxPoints:]
	}
	return out
}

func firstLine(cmd ...string) string {
	out, _ := runCmd(cmd[0], cmd[1:]...)
	return strings.Split(out, "\n")[0]
}

func getXrayConfigJSON() (interface{}, error) {
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return nil, err
	}
	var cfg interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

