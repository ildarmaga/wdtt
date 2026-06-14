package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// WdttSharePayload — JSON внутри base64 для wdtt:// ссылок.
type WdttSharePayload struct {
	Vpn  string `json:"vpn,omitempty"`  // название VPN (заголовок подписки)
	Name string `json:"name,omitempty"` // комментарий пользователя
	Ps   string `json:"ps,omitempty"`   // legacy: то же что name
	IP   string `json:"ip"`
	Dtls int    `json:"dtls"`
	Pass string `json:"pass"`
	Did  string `json:"did,omitempty"`
	Hash string `json:"hash,omitempty"`
	Sub  string `json:"sub,omitempty"`
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
	p.Sub = legacy.Sub
	return p, nil
}

func buildWdttShareLink(serverIP, password, remark, vpnTitle, deviceID, _ string, entry *PasswordEntry, inbound WdttInboundConfig, subURL string) (string, error) {
	inbound.normalize()
	host := strings.TrimSpace(inbound.ServerHost)
	if host == "" {
		host = serverIP
	}
	dtls, _, _ := resolveUserPorts(entry, inbound)
	userName := strings.TrimSpace(remark)
	if userName == "" && entry != nil {
		userName = strings.TrimSpace(entry.Comment)
	}
	if userName == "" {
		userName = inbound.Remark
	}
	vpnName := strings.TrimSpace(vpnTitle)
	if vpnName == "" {
		vpnName = strings.TrimSpace(inbound.Tag)
	}
	// hash намеренно не в base64-ссылке (v1.3.3+): хеши — в colon/qwdtt или поле VK_HASH у пользователя.
	// name — имя профиля; ps не дублируем (legacy-клиенты читают name или ps при decode).
	return encodeWdttShareLink(WdttSharePayload{
		Vpn:  vpnName,
		Name: userName,
		IP:   host,
		Dtls: dtls,
		Pass: password,
		Did:  strings.TrimSpace(deviceID),
		Sub:  strings.TrimSpace(subURL),
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
