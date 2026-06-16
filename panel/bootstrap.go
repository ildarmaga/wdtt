package panel

import "sync"

var bootstrapOnce sync.Once
var bootstrapErr error

// BootstrapDB создаёт panel.db и seed пользователей до старта VPN-сервера.
// Без этого server.Run() может упасть на [WRAP] нет активных паролей (panel.Run в goroutine).
func BootstrapDB() error {
	bootstrapOnce.Do(func() {
		if err := initPanelDB(); err != nil {
			bootstrapErr = err
			return
		}
		ensureLegacySettingsImported()
		ensureDefaultWdttData()
	})
	return bootstrapErr
}
