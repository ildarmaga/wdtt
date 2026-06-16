package panel

import (
	"sync"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

var (
	passwordsCacheMu  sync.RWMutex
	passwordsCache    *PasswordsDB
	passwordsCacheRev int64
)

func invalidatePasswordsCache() {
	passwordsCacheMu.Lock()
	passwordsCache = nil
	passwordsCacheRev = 0
	passwordsCacheMu.Unlock()
}

func paneldbLoadUsersRev() (int64, error) {
	if !panelDBEnabled() {
		return 0, nil
	}
	return paneldb.LoadUsersRev(panelDB)
}

func loadPasswordsCached() (*PasswordsDB, error) {
	if !panelDBEnabled() {
		return loadPasswords()
	}
	rev, err := paneldbLoadUsersRev()
	if err != nil {
		return loadPasswords()
	}
	passwordsCacheMu.RLock()
	if passwordsCache != nil && passwordsCacheRev == rev {
		db := passwordsCache
		passwordsCacheMu.RUnlock()
		return db, nil
	}
	passwordsCacheMu.RUnlock()

	db, err := loadPasswords()
	if err != nil {
		return nil, err
	}
	passwordsCacheMu.Lock()
	passwordsCache = db
	passwordsCacheRev = rev
	passwordsCacheMu.Unlock()
	return db, nil
}
