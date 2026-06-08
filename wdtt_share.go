package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type wdttSharePayload struct {
	V    string `json:"v"`
	Ps   string `json:"ps,omitempty"`
	Tag  string `json:"tag,omitempty"`
	Add  string `json:"add"`
	Dtls int    `json:"dtls"`
	Wg   int    `json:"wg"`
	Lp   int    `json:"lp,omitempty"`
	Id   string `json:"id"`
	Did  string `json:"did,omitempty"`
	Hash string `json:"hash,omitempty"`
}

func encodeWdttShareLink(p wdttSharePayload) (string, error) {
	if p.V == "" {
		p.V = "1"
	}
	if p.Add == "" || p.Id == "" || p.Dtls <= 0 || p.Wg <= 0 {
		return "", fmt.Errorf("incomplete wdtt share payload")
	}
	if p.Lp <= 0 {
		p.Lp = 9000
	}
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return "wdtt://" + base64.StdEncoding.EncodeToString(data), nil
}

func buildWdttShareLinkFromPorts(srvIP, ports, password, remark, vkHash string) (string, error) {
	pts := strings.Split(ports, ",")
	dtls, wg, lp := "56000", "56001", "9000"
	if len(pts) >= 1 && strings.TrimSpace(pts[0]) != "" {
		dtls = strings.TrimSpace(pts[0])
	}
	if len(pts) >= 2 && strings.TrimSpace(pts[1]) != "" {
		wg = strings.TrimSpace(pts[1])
	}
	if len(pts) >= 3 && strings.TrimSpace(pts[2]) != "" {
		lp = strings.TrimSpace(pts[2])
	}
	dtlsN, _ := strconv.Atoi(dtls)
	wgN, _ := strconv.Atoi(wg)
	lpN, _ := strconv.Atoi(lp)
	return encodeWdttShareLink(wdttSharePayload{
		V:    "1",
		Ps:   remark,
		Tag:  "wdtt-in",
		Add:  srvIP,
		Dtls: dtlsN,
		Wg:   wgN,
		Lp:   lpN,
		Id:   password,
		Hash: vkHash,
	})
}
