package sharelink

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func Encode(p Payload) (string, error) {
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

func Decode(link string) (Payload, error) {
	var p Payload
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, "wdtt://") {
		return p, fmt.Errorf("неверный префикс ссылки")
	}
	raw := strings.TrimPrefix(link, "wdtt://")
	if strings.Contains(raw, ":") && !strings.Contains(raw, "=") && len(raw) < 200 {
		return p, fmt.Errorf("устаревший colon-формат")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return p, fmt.Errorf("не удалось декодировать ссылку: %w", err)
		}
	}
	var legacy struct {
		Vpn  string `json:"vpn"`
		Name string `json:"name"`
		Ps   string `json:"ps"`
		IP   string `json:"ip"`
		Add  string `json:"add"`
		Dtls int    `json:"dtls"`
		Pass string `json:"pass"`
		Id   string `json:"id"`
		Did  string `json:"did"`
		Hash string `json:"hash"`
		Sub  string `json:"sub"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return p, fmt.Errorf("неверный JSON в ссылке: %w", err)
	}
	p.Vpn = strings.TrimSpace(legacy.Vpn)
	p.Name = strings.TrimSpace(firstNonEmpty(legacy.Name, legacy.Ps))
	p.Ps = p.Name
	p.IP = strings.TrimSpace(firstNonEmpty(legacy.IP, legacy.Add))
	p.Dtls = legacy.Dtls
	p.Pass = strings.TrimSpace(firstNonEmpty(legacy.Pass, legacy.Id))
	p.Did = legacy.Did
	p.Hash = legacy.Hash
	p.Sub = legacy.Sub
	return p, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
