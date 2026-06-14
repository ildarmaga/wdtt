package paneldb

const DefaultPath = "/etc/wdtt/panel.db"

const (
	DefaultMaxDevices = 1
	MaxDevicesLimit   = 20
)

// Store — wdtt_users + wdtt_devices + wdtt_global (ядро VPN-данных).
// Настройки панели — panel_config (см. panel_config.go).
type Store struct {
	MainPassword string
	AdminID      string
	BotToken     string
	Users        map[string]*User
	Devices      map[string]*Device
}

type User struct {
	DeviceID      string
	DeviceIDs     []string
	MaxDevices    int
	ExpiresAt     int64
	DownBytes     int64
	UpBytes       int64
	TotalBytes    int64
	MaxDownMBps   float64
	MaxUpMBps     float64
	IsDeactivated bool
	Comment       string
	Ports         string
	VkHash        string
	SubID         string
	LastSeenAt    int64
}

type Device struct {
	DeviceID string
	IP       string
	PrivKey  string
	PubKey   string
}

// SaveOptions — поведение SaveStore.
type SaveOptions struct {
	// PreserveSubIDs: если SubID пуст у пользователя — оставить из БД (сервер).
	PreserveSubIDs bool
}

func NewStore() *Store {
	return &Store{
		Users:   make(map[string]*User),
		Devices: make(map[string]*Device),
	}
}
