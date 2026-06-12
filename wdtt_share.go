package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type wdttSharePayload struct {
	Ps   string `json:"ps,omitempty"`
	IP   string `json:"ip"`
	Dtls int    `json:"dtls"`
	Pass string `json:"pass"`
	Did  string `json:"did,omitempty"`
	Hash string `json:"hash,omitempty"`
}

func encodeWdttShareLink(p wdttSharePayload) (string, error) {
	if strings.TrimSpace(p.IP) == "" || strings.TrimSpace(p.Pass) == "" || p.Dtls <= 0 {
		return "", fmt.Errorf("incomplete wdtt share payload")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return "wdtt://" + base64.StdEncoding.EncodeToString(data), nil
}

func buildWdttShareLinkFromPorts(srvIP, ports, password, remark, vkHash string) (string, error) {
	pts := strings.Split(ports, ",")
	dtls := "56000"
	if len(pts) >= 1 && strings.TrimSpace(pts[0]) != "" {
		dtls = strings.TrimSpace(pts[0])
	}
	dtlsN, _ := strconv.Atoi(dtls)
	return encodeWdttShareLink(wdttSharePayload{
		Ps:   remark,
		IP:   srvIP,
		Dtls: dtlsN,
		Pass: password,
		Hash: vkHash,
	})
}
