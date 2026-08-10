package sharelink

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/vkhash"
)

// ColonLinkParams — wdtt://host:dtls:wg:local:pass:hash[,hash…][#name]
type ColonLinkParams struct {
	Host      string
	Password  string
	Name      string
	VkHash    string
	DtlsPort  int
	WgPort    int
	LocalPort int
	WithName  bool
	HashLimit int // 0 = vkhash.Max; 1 для iOS
}

// BuildColonLink формирует colon-ссылку (как wdttBuildColonLink в UI).
func BuildColonLink(p ColonLinkParams) string {
	host := strings.TrimSpace(p.Host)
	password := strings.TrimSpace(p.Password)
	if host == "" || password == "" {
		return ""
	}
	dtls := p.DtlsPort
	if dtls <= 0 {
		dtls = 56000
	}
	wg := p.WgPort
	if wg <= 0 {
		wg = 56001
	}
	limit := p.HashLimit
	if limit <= 0 {
		limit = vkhash.Max
	}
	hash := vkhash.FormatForLink(p.VkHash, vkhash.FormatBare, limit)
	link := fmt.Sprintf("wdtt://%s:%d:%d:%d:%s:%s", host, dtls, wg, p.LocalPort, password, hash)
	if p.WithName {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = "WDTT"
		}
		link += "#" + name
	}
	return link
}

// QwdttLinkParams — qwdtt://config?…
type QwdttLinkParams struct {
	Host     string
	Password string
	Name     string
	VkHash   string
	Port     int // client port, default 9000
	Workers  int // default 18
}

// BuildQwdttLink формирует qwdtt-ссылку (как wdttBuildQwdttLink в UI).
// pass — последний параметр, без percent-encoding (& и @ в пароле сохраняются).
func BuildQwdttLink(p QwdttLinkParams) string {
	host := strings.TrimSpace(p.Host)
	password := strings.TrimSpace(p.Password)
	if host == "" || password == "" {
		return ""
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "WDTT"
	}
	port := p.Port
	if port <= 0 {
		port = 9000
	}
	workers := p.Workers
	if workers <= 0 {
		workers = 18
	}
	q := url.Values{}
	q.Set("name", name)
	q.Set("peer", host)
	q.Set("hashes", vkhash.FormatForLink(p.VkHash, vkhash.FormatBare, 0))
	q.Set("workers", strconv.Itoa(workers))
	q.Set("port", strconv.Itoa(port))
	return "qwdtt://config?" + q.Encode() + "&pass=" + password
}

// BuildPanelLink — ссылка из панели/API: vpn, name, sub, hash (если задан).
func BuildPanelLink(p PanelLinkParams) (string, error) {
	dtls := p.DtlsPort
	if dtls <= 0 {
		dtls = 56000
	}
	pl := Payload{
		Vpn:  strings.TrimSpace(p.VpnName),
		Name: strings.TrimSpace(p.UserName),
		IP:   strings.TrimSpace(p.Host),
		Dtls: dtls,
		Raw:  p.RawPort,
		Pass: strings.TrimSpace(p.Password),
		Did:  strings.TrimSpace(p.DeviceID),
		Sub:  strings.TrimSpace(p.SubURL),
		WbRoom: strings.TrimSpace(p.WbRoom),
	}
	if h := strings.TrimSpace(p.VkHash); h != "" {
		pl.Hash = vkhash.FormatForLink(h, vkhash.FormatBare, vkhash.Max)
	}
	return Encode(pl)
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
		Raw:  p.RawPort,
		Pass: strings.TrimSpace(p.Password),
	}
	if h := strings.TrimSpace(p.VkHash); h != "" {
		pl.Hash = h
	}
	return Encode(pl)
}
