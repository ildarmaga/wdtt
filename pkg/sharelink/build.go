package sharelink

import "strings"

// BuildPanelLink — ссылка из панели/API: vpn, name, sub; без hash и без ps.
func BuildPanelLink(p PanelLinkParams) (string, error) {
	dtls := p.DtlsPort
	if dtls <= 0 {
		dtls = 56000
	}
	return Encode(Payload{
		Vpn:  strings.TrimSpace(p.VpnName),
		Name: strings.TrimSpace(p.UserName),
		IP:   strings.TrimSpace(p.Host),
		Dtls: dtls,
		Pass: strings.TrimSpace(p.Password),
		Did:  strings.TrimSpace(p.DeviceID),
		Sub:  strings.TrimSpace(p.SubURL),
	})
}

// BuildBotLink — ссылка из Telegram-бота (legacy ps + optional hash).
func BuildBotLink(p BotLinkParams) (string, error) {
	dtls := p.DtlsPort
	if dtls <= 0 {
		dtls = 56000
	}
	pl := Payload{
		Ps:   strings.TrimSpace(p.Remark),
		IP:   strings.TrimSpace(p.Host),
		Dtls: dtls,
		Pass: strings.TrimSpace(p.Password),
	}
	if h := strings.TrimSpace(p.VkHash); h != "" {
		pl.Hash = h
	}
	return Encode(pl)
}
