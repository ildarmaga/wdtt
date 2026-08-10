package sharelink

// Payload — JSON внутри base64 для wdtt:// ссылок.
type Payload struct {
	Vpn  string `json:"vpn,omitempty"`  // заголовок VPN / подписки
	Name string `json:"name,omitempty"` // комментарий пользователя
	Ps   string `json:"ps,omitempty"`   // legacy: то же что name (только при decode старых ссылок)
	IP   string `json:"ip"`
	Dtls int    `json:"dtls"`
	Raw  int    `json:"raw,omitempty"` // direct RAW UDP (WRAP); 0 = клиент использует DTLS+3
	Pass string `json:"pass"`
	Did  string `json:"did,omitempty"`
	Hash string `json:"hash,omitempty"` // legacy / bot; в panel API base64 не кладём
	Sub     string `json:"sub,omitempty"`
	WbRoom  string `json:"wb_room,omitempty"`
}

// PanelLinkParams — base64-ссылка из панели (vpn, name, sub, optional hash).
type PanelLinkParams struct {
	Host     string
	Password string
	UserName string
	VpnName  string
	DeviceID string
	VkHash   string
	SubURL   string
	WbRoom   string
	DtlsPort int
	RawPort  int // 0 = omit (клиент DTLS+3)
}

// BotLinkParams — ссылка из Telegram-бота сервера (может включать hash).
type BotLinkParams struct {
	Host     string
	Password string
	Remark   string
	VkHash   string
	DtlsPort int
	RawPort  int
}
