package panel

import (
	"strings"

	"github.com/ildarmaga/wdtt/pkg/sharelink"
)

// WdttSharePayload — алиас для совместимости шаблонов/API.
type WdttSharePayload = sharelink.Payload

func encodeWdttShareLink(p WdttSharePayload) (string, error) {
	return sharelink.Encode(p)
}

func decodeWdttShareLink(link string) (WdttSharePayload, error) {
	return sharelink.Decode(link)
}

func buildWdttShareLink(serverIP, password, remark, vpnTitle, deviceID, vkHash string, entry *PasswordEntry, inbound WdttInboundConfig, subURL string) (string, error) {
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
	return sharelink.BuildPanelLink(sharelink.PanelLinkParams{
		Host:     host,
		Password: password,
		UserName: userName,
		VpnName:  vpnName,
		DeviceID: deviceID,
		VkHash:   vkHash,
		SubURL:   subURL,
		DtlsPort: dtls,
	})
}

// buildAllSubscriptionLinks — все форматы из вкладки «Подключения» профиля.
func buildAllSubscriptionLinks(linkHost, password, remark, vpnTitle string, entry *PasswordEntry, inbound WdttInboundConfig, subURL string) (links, titles []string, err error) {
	vkHash := ""
	if entry != nil {
		vkHash = entry.VkHash
	}
	jsonLink, err := buildWdttShareLink(linkHost, password, remark, vpnTitle, "", vkHash, entry, inbound, subURL)
	if err != nil {
		return nil, nil, err
	}
	inbound.normalize()
	host := strings.TrimSpace(inbound.ServerHost)
	if host == "" {
		host = linkHost
	}
	dtls, wg, client := resolveUserPorts(entry, inbound)
	name := strings.TrimSpace(remark)
	if name == "" && entry != nil {
		name = strings.TrimSpace(entry.Comment)
	}
	if name == "" {
		name = "WDTT"
	}
	base := sharelink.ColonLinkParams{
		Host: host, Password: password, Name: name, VkHash: vkHash,
		DtlsPort: dtls, WgPort: wg,
	}
	candidates := []struct {
		title string
		link  string
	}{
		{"WDTT JSON", jsonLink},
		{"iOS — VK Turn Proxy", sharelink.BuildColonLink(sharelink.ColonLinkParams{
			Host: base.Host, Password: base.Password, Name: base.Name, VkHash: base.VkHash,
			DtlsPort: base.DtlsPort, WgPort: base.WgPort, HashLimit: 1,
		})},
		{"Android — WDTT", sharelink.BuildColonLink(sharelink.ColonLinkParams{
			Host: base.Host, Password: base.Password, Name: base.Name, VkHash: base.VkHash,
			DtlsPort: base.DtlsPort, WgPort: base.WgPort, LocalPort: client,
		})},
		{"PWDTT — Desktop", sharelink.BuildColonLink(sharelink.ColonLinkParams{
			Host: base.Host, Password: base.Password, Name: base.Name, VkHash: base.VkHash,
			DtlsPort: base.DtlsPort, WgPort: base.WgPort, WithName: true,
		})},
		{"WDTT — Windows", sharelink.BuildColonLink(sharelink.ColonLinkParams{
			Host: base.Host, Password: base.Password, Name: base.Name, VkHash: base.VkHash,
			DtlsPort: base.DtlsPort, WgPort: base.WgPort, WithName: true,
		})},
		{"qWDTT", sharelink.BuildQwdttLink(sharelink.QwdttLinkParams{
			Host: host, Password: password, Name: name, VkHash: vkHash, Port: client,
		})},
	}
	seen := make(map[string]struct{})
	for _, c := range candidates {
		link := strings.TrimSpace(c.link)
		if link == "" {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		links = append(links, link)
		titles = append(titles, c.title)
	}
	return links, titles, nil
}
