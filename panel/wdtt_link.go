package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// WdttSharePayload — JSON внутри base64 для wdtt:// ссылок.
type WdttSharePayload struct {
	Ps   string `json:"ps,omitempty"`
	IP   string `json:"ip"`
	Dtls int    `json:"dtls"`
	Pass string `json:"pass"`
	Did  string `json:"did,omitempty"`
	Hash string `json:"hash,omitempty"`
}

func encodeWdttShareLink(p WdttSharePayload) (string, error) {
	if strings.TrimSpace(p.IP) == "" {
		return "", fmt.Errorf("не указан адрес сервера")
	}
	if strings.TrimSpace(p.Pass) == "" {
		return "", fmt.Errorf("не указан пароль")
	}
	if p.Dtls <= 0 {
		return "", fmt.Errorf("неверный порт DTLS")
	}
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return "wdtt://" + base64.StdEncoding.EncodeToString(data), nil
}

func decodeWdttShareLink(link string) (WdttSharePayload, error) {
	var p WdttSharePayload
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, "wdtt://") {
		return p, fmt.Errorf("неверный префикс ссылки")
	}
	raw := strings.TrimPrefix(link, "wdtt://")
	if strings.Contains(raw, ":") && !strings.Contains(raw, "=") && len(raw) < 200 {
		return p, fmt.Errorf("устаревший формат ссылки")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return p, fmt.Errorf("не удалось декодировать ссылку: %w", err)
		}
	}
	var legacy struct {
		Ps   string `json:"ps"`
		IP   string `json:"ip"`
		Add  string `json:"add"`
		Dtls int    `json:"dtls"`
		Pass string `json:"pass"`
		Id   string `json:"id"`
		Did  string `json:"did"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return p, fmt.Errorf("неверный JSON в ссылке: %w", err)
	}
	p.Ps = legacy.Ps
	p.IP = strings.TrimSpace(legacy.IP)
	if p.IP == "" {
		p.IP = strings.TrimSpace(legacy.Add)
	}
	p.Dtls = legacy.Dtls
	p.Pass = strings.TrimSpace(legacy.Pass)
	if p.Pass == "" {
		p.Pass = strings.TrimSpace(legacy.Id)
	}
	p.Did = legacy.Did
	p.Hash = legacy.Hash
	return p, nil
}

func buildWdttShareLink(serverIP, password, remark, tag, deviceID, vkHash string, entry *PasswordEntry, inbound WdttInboundConfig) (string, error) {
	inbound.normalize()
	host := strings.TrimSpace(inbound.ServerHost)
	if host == "" {
		host = serverIP
	}
	dtls, _, _ := resolveUserPorts(entry, inbound)
	ps := strings.TrimSpace(remark)
	if ps == "" && entry != nil {
		ps = strings.TrimSpace(entry.Comment)
	}
	if ps == "" {
		ps = inbound.Remark
	}
	if entry != nil && vkHash == "" {
		vkHash = entry.VkHash
	}
	return encodeWdttShareLink(WdttSharePayload{
		Ps:   ps,
		IP:   host,
		Dtls: dtls,
		Pass: password,
		Did:  strings.TrimSpace(deviceID),
		Hash: vkHash,
	})
}
