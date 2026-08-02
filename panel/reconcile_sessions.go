package panel

import "github.com/ildarmaga/wdtt/pkg/paneldb"

// reconcileUserSessionFields выравнивает wdtt_users.vk_hash с активными vk_calls.
func reconcileUserSessionFields() {
	if !panelDBEnabled() {
		return
	}
	if n, err := paneldb.ReconcileVKHashesSync(panelDB); err == nil && n > 0 {
		invalidatePasswordsCache()
	}
}
