package server

import (
	"strconv"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/sharelink"
)

func buildWdttShareLinkFromPorts(srvIP, ports, password, remark, vkHash string) (string, error) {
	pts := strings.Split(ports, ",")
	dtls := "56000"
	if len(pts) >= 1 && strings.TrimSpace(pts[0]) != "" {
		dtls = strings.TrimSpace(pts[0])
	}
	dtlsN, _ := strconv.Atoi(dtls)
	return sharelink.BuildBotLink(sharelink.BotLinkParams{
		Host:     srvIP,
		Password: password,
		Remark:   remark,
		VkHash:   vkHash,
		DtlsPort: dtlsN,
	})
}
