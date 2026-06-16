package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ProcessInfo struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	Mem      uint64  `json:"mem"`
	CPU      float64 `json:"cpu"`
	Killable bool    `json:"killable"`
}

var (
	procListMu        sync.Mutex
	lastProcCPU       map[int]uint64
	lastTotalCPUTicks uint64
	hasProcCPUSample  bool
)

func getProcessList(limit int) []ProcessInfo {
	if limit <= 0 {
		limit = 40
	}
	entries := collectProcessEntries()
	totalCPU := readTotalCPUTicks()

	procListMu.Lock()
	defer procListMu.Unlock()

	numCPU := cachedCPUCores
	if numCPU <= 0 {
		numCPU = 1
	}

	var deltaTotal float64
	if hasProcCPUSample {
		deltaTotal = float64(totalCPU - lastTotalCPUTicks)
	}
	nextSamples := make(map[int]uint64, len(entries))

	out := make([]ProcessInfo, 0, len(entries))
	for _, e := range entries {
		ticks := e.utime + e.stime
		nextSamples[e.pid] = ticks
		cpu := 0.0
		if hasProcCPUSample && deltaTotal > 0 {
			if prev, ok := lastProcCPU[e.pid]; ok {
				cpu = float64(ticks-prev) / deltaTotal * 100 * float64(numCPU)
				if cpu < 0 {
					cpu = 0
				}
			}
		}
		out = append(out, ProcessInfo{
			PID:      e.pid,
			Name:     e.name,
			Mem:      e.mem,
			CPU:      roundProcessCPU(cpu),
			Killable: isProcessKillable(e.pid),
		})
	}

	lastProcCPU = nextSamples
	lastTotalCPUTicks = totalCPU
	hasProcCPUSample = true

	sort.Slice(out, func(i, j int) bool {
		return out[i].Mem > out[j].Mem
	})
	if len(out) > limit {
		out = out[:limit]
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PID < out[j].PID
	})
	return out
}

type procEntry struct {
	pid           int
	name          string
	mem           uint64
	utime, stime  uint64
}

func collectProcessEntries() []procEntry {
	dir, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := make([]procEntry, 0, len(dir))
	for _, de := range dir {
		if !de.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(de.Name())
		if err != nil || pid <= 0 {
			continue
		}
		name, mem, utime, stime, ok := readProcEntry(pid)
		if !ok || mem == 0 {
			continue
		}
		out = append(out, procEntry{pid: pid, name: name, mem: mem, utime: utime, stime: stime})
	}
	return out
}

func readProcEntry(pid int) (name string, mem, utime, stime uint64, ok bool) {
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return "", 0, 0, 0, false
	}
	comm, ut, st, err := parseProcStat(string(data))
	if err != nil {
		return "", 0, 0, 0, false
	}
	mem = readProcRSS(pid)
	if mem == 0 {
		return "", 0, 0, 0, false
	}
	display := processDisplayName(pid, comm)
	return display, mem, ut, st, true
}

func parseProcStat(raw string) (comm string, utime, stime uint64, err error) {
	start := strings.Index(raw, "(")
	end := strings.LastIndex(raw, ")")
	if start < 0 || end <= start {
		return "", 0, 0, fmt.Errorf("bad stat")
	}
	comm = raw[start+1 : end]
	fields := strings.Fields(raw[end+2:])
	if len(fields) < 15 {
		return "", 0, 0, fmt.Errorf("short stat")
	}
	utime, err = strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return "", 0, 0, err
	}
	stime, err = strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return "", 0, 0, err
	}
	return comm, utime, stime, nil
}

func readProcRSS(pid int) uint64 {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			kb, _ := strconv.ParseUint(f[1], 10, 64)
			return kb * 1024
		}
	}
	return 0
}

func processDisplayName(pid int, comm string) string {
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err == nil && len(cmdline) > 0 {
		parts := strings.Split(string(cmdline), "\x00")
		if len(parts) > 0 && parts[0] != "" {
			base := filepath.Base(parts[0])
			if base != "" {
				return base
			}
		}
	}
	return strings.TrimSpace(comm)
}

func readTotalCPUTicks() uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line := strings.Split(string(data), "\n")[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}
	var total uint64
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
	}
	return total
}

func roundProcessCPU(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func isProcessKillable(pid int) bool {
	if pid <= 1 || pid == os.Getpid() {
		return false
	}
	return true
}

func killProcess(pid int) error {
	if !isProcessKillable(pid) {
		return fmt.Errorf("process %d cannot be terminated", pid)
	}
	if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err != nil {
		return fmt.Errorf("process not found")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); os.IsNotExist(err) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}
