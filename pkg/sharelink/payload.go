package sharelink

// Payload — JSON внутри base64 для wdtt:// ссылок.
type Payload struct {
	Vpn  string `json:"vpn,omitempty"`  // заголовок VPN / подписки
	Name string `json:"name,omitempty"` // комментарий пользователя
	Ps   string `json:"ps,omitempty"`   // legacy: то же что name (только при decode старых ссылок)
	IP   string `json:"ip"`
	Dtls int    `json:"dtls"`
	Pass string `json:"pass"`
	Did  string `json:"did,omitempty"`
	Hash string `json:"hash,omitempty"` // legacy / bot; в panel API base64 не кладём
	Sub  string `json:"sub,omitempty"`
}

// PanelLinkParams — base64-ссылка из панели (без hash, без ps).
type PanelLinkParams struct {
	Host     string
	Password string
	UserName string
	VpnName  string
	DeviceID string
	SubURL   string
	DtlsPort int
}

// BotLinkParams — ссылка из Telegram-бота сервера (может включать hash).
type BotLinkParams struct {
	Host     string
	Password string
	Remark   string
	VkHash   string
	DtlsPort int
}
