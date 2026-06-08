package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultXrayAPIPort = 62789

type XrayLogEntry struct {
	DateTime    time.Time `json:"DateTime"`
	FromAddress string    `json:"FromAddress"`
	ToAddress   string    `json:"ToAddress"`
	Inbound     string    `json:"Inbound"`
	Outbound    string    `json:"Outbound"`
	Email       string    `json:"Email"`
	Event       int       `json:"Event"`
}

type OutboundTraffic struct {
	Tag   string `json:"tag"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Total int64  `json:"total"`
}

func xrayAPIAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", xrayAPIPortFromConfig())
}

func xrayAPIPortFromConfig() int {
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return defaultXrayAPIPort
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultXrayAPIPort
	}
	inbounds, _ := cfg["inbounds"].([]interface{})
	for _, ib := range inbounds {
		m, _ := ib.(map[string]interface{})
		tag, _ := m["tag"].(string)
		if tag != "api" {
			continue
		}
		switch p := m["port"].(type) {
		case float64:
			return int(p)
		case int:
			return p
		}
	}
	return defaultXrayAPIPort
}

func getAccessLogPath() (string, error) {
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return "", err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	logCfg, _ := cfg["log"].(map[string]interface{})
	access, _ := logCfg["access"].(string)
	if access == "" || access == "none" {
		return "", fmt.Errorf("access log not configured")
	}
	return access, nil
}

func getDefaultLogOutboundTags() (freedoms, blackholes []string) {
	data, err := os.ReadFile(xrayConfigPath)
	if err != nil {
		return []string{"direct"}, []string{"blocked"}
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return []string{"direct"}, []string{"blocked"}
	}
	outbounds, _ := cfg["outbounds"].([]interface{})
	for _, ob := range outbounds {
		m, _ := ob.(map[string]interface{})
		tag, _ := m["tag"].(string)
		if tag == "" {
			continue
		}
		switch m["protocol"] {
		case "freedom":
			freedoms = append(freedoms, tag)
		case "blackhole":
			blackholes = append(blackholes, tag)
		}
	}
	if len(freedoms) == 0 {
		freedoms = []string{"direct"}
	}
	if len(blackholes) == 0 {
		blackholes = []string{"blocked"}
	}
	return freedoms, blackholes
}

func logEntryContains(line string, suffixes []string) bool {
	for _, sfx := range suffixes {
		if strings.Contains(line, sfx+"]") {
			return true
		}
	}
	return false
}

func isFilterFalse(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return !x
	case string:
		return x == "false"
	}
	return false
}

func getXrayLogs(count int, filter string, showDirect, showBlocked, showProxy interface{}) []XrayLogEntry {
	const (
		directEvent = iota
		blockedEvent
		proxiedEvent
	)

	freedoms, blackholes := getDefaultLogOutboundTags()
	pathToAccessLog, err := getAccessLogPath()
	if err != nil {
		return nil
	}

	file, err := os.Open(pathToAccessLog)
	if err != nil {
		return nil
	}
	defer file.Close()

	var entries []XrayLogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, "api -> api") {
			continue
		}
		if !strings.Contains(line, " accepted ") && !strings.Contains(line, " rejected ") {
			continue
		}
		if filter != "" && !strings.Contains(line, filter) {
			continue
		}

		var entry XrayLogEntry
		parts := strings.Fields(line)
		for i, part := range parts {
			if i == 0 && len(parts) > 1 {
				dateTime, err := time.ParseInLocation("2006/01/02 15:04:05.999999", parts[0]+" "+parts[1], time.Local)
				if err != nil {
					continue
				}
				entry.DateTime = dateTime.UTC()
			}
			if part == "from" && i+1 < len(parts) {
				entry.FromAddress = strings.TrimLeft(parts[i+1], "/")
			} else if part == "accepted" && i+1 < len(parts) {
				entry.ToAddress = strings.TrimLeft(parts[i+1], "/")
			} else if strings.HasPrefix(part, "[") {
				entry.Inbound = part[1:]
			} else if strings.HasSuffix(part, "]") {
				entry.Outbound = part[:len(part)-1]
			} else if part == "email:" && i+1 < len(parts) {
				entry.Email = parts[i+1]
			}
		}

		if logEntryContains(line, freedoms) {
			if isFilterFalse(showDirect) {
				continue
			}
			entry.Event = directEvent
		} else if logEntryContains(line, blackholes) {
			if isFilterFalse(showBlocked) {
				continue
			}
			entry.Event = blockedEvent
		} else {
			if isFilterFalse(showProxy) {
				continue
			}
			entry.Event = proxiedEvent
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil
	}
	if count > 0 && len(entries) > count {
		entries = entries[len(entries)-count:]
	}
	// Newest first (как 3x-ui).
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

func queryXrayStats() (map[string]int64, error) {
	bin := xrayBinary()
	if bin == "" {
		return nil, fmt.Errorf("xray binary not found")
	}
	out, err := runCmd(bin, "api", "statsquery", "--server="+xrayAPIAddr())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Stat []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"stat"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(resp.Stat))
	for _, s := range resp.Stat {
		result[s.Name] = s.Value
	}
	return result, nil
}

func getOutboundsTraffic() ([]OutboundTraffic, error) {
	stats, err := queryXrayStats()
	if err != nil {
		return []OutboundTraffic{}, nil
	}
	re := regexp.MustCompile(`^outbound>>>([^>]+)>>>traffic>>>(uplink|downlink)$`)
	byTag := map[string]*OutboundTraffic{}
	for name, val := range stats {
		m := re.FindStringSubmatch(name)
		if len(m) != 3 {
			continue
		}
		tag := m[1]
		if tag == "api" {
			continue
		}
		t, ok := byTag[tag]
		if !ok {
			t = &OutboundTraffic{Tag: tag}
			byTag[tag] = t
		}
		if m[2] == "uplink" {
			t.Up = val
		} else {
			t.Down = val
		}
	}
	result := make([]OutboundTraffic, 0, len(byTag))
	for _, t := range byTag {
		t.Total = t.Up + t.Down
		result = append(result, *t)
	}
	return result, nil
}

func resetOutboundStats(tag string) error {
	bin := xrayBinary()
	if bin == "" {
		return fmt.Errorf("xray binary not found")
	}
	addr := xrayAPIAddr()
	tags := []string{}
	if tag == "-alltags-" || tag == "" {
		traffics, _ := getOutboundsTraffic()
		for _, t := range traffics {
			tags = append(tags, t.Tag)
		}
	} else {
		tags = []string{tag}
	}
	for _, tg := range tags {
		for _, dir := range []string{"uplink", "downlink"} {
			name := fmt.Sprintf("outbound>>>%s>>>traffic>>>%s", tg, dir)
			_, _ = runCmd(bin, "api", "stats", "--server="+addr, "-name="+name, "-reset")
		}
	}
	return nil
}

func parseXrayLogsCount(r *http.Request, basePath string) int {
	prefix := basePath + "panel/api/server/xraylogs/"
	s := strings.TrimPrefix(r.URL.Path, prefix)
	s = strings.Trim(s, "/")
	c, _ := strconv.Atoi(s)
	if c <= 0 {
		return 20
	}
	return c
}
