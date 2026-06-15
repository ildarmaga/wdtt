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
	return sharelink.BuildPanelLink(sharelink.PanelLinkParams{
		Host:     host,
		Password: password,
		UserName: userName,
		VpnName:  vpnName,
		DeviceID: deviceID,
		SubURL:   subURL,
		DtlsPort: dtls,
	})
}
