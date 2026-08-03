package panel

import "github.com/ildarmaga/wdtt/pkg/paneldb"

// reconcileUserSessionFields: live vk_calls → профиль, finishing снимаем;
// ручные хеши (без строк в vk_calls) сохраняются.
func reconcileUserSessionFields() {
	if !panelDBEnabled() {
		return
	}
	if n, err := paneldb.ReconcileVKHashesSync(panelDB); err == nil && n > 0 {
		invalidatePasswordsCache()
	}
}
