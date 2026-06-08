package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// WdttSharePayload — формат как vmess:// в 3x-ui: JSON внутри base64.
type WdttSharePayload struct {
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

func encodeWdttShareLink(p WdttSharePayload) (string, error) {
	if p.V == "" {
		p.V = "1"
	}
	if p.Add == "" {
		return "", fmt.Errorf("не указан адрес сервера")
	}
	if p.Id == "" {
		return "", fmt.Errorf("не указан пароль")
	}
	if p.Dtls <= 0 || p.Wg <= 0 {
		return "", fmt.Errorf("неверные порты")
	}
	if p.Lp <= 0 {
		p.Lp = defaultClientPort
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
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("неверный JSON в ссылке: %w", err)
	}
	return p, nil
}

func buildWdttShareLink(serverIP, password, remark, tag, deviceID, vkHash string, entry *PasswordEntry, inbound WdttInboundConfig) (string, error) {
	inbound.normalize()
	host := strings.TrimSpace(inbound.ServerHost)
	if host == "" {
		host = serverIP
	}
	dtls, wg, client := resolveUserPorts(entry, inbound)
	ps := strings.TrimSpace(remark)
	if ps == "" && entry != nil {
		ps = strings.TrimSpace(entry.Comment)
	}
	if ps == "" {
		ps = inbound.Remark
	}
	if tag == "" {
		tag = inbound.Tag
	}
	if entry != nil && vkHash == "" {
		vkHash = entry.VkHash
	}
	return encodeWdttShareLink(WdttSharePayload{
		V:    "1",
		Ps:   ps,
		Tag:  tag,
		Add:  host,
		Dtls: dtls,
		Wg:   wg,
		Lp:   client,
		Id:   password,
		Did:  "",
		Hash: vkHash,
	})
}
