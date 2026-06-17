package panel

import (
	"fmt"
	"strings"

	"github.com/ildarmaga/wdtt/pkg/paneldb"
)

func listVKCreatorSessions() ([]vkCreatorSession, error) {
	if !panelDBEnabled() {
		return nil, fmt.Errorf("база панели недоступна")
	}
	calls, err := paneldb.ListVKCalls(panelDB)
	if err != nil {
		return nil, err
	}
	out := make([]vkCreatorSession, 0, len(calls))
	for _, c := range calls {
		out = append(out, vkCreatorSession{
			Password: c.Password, JoinLink: c.JoinLink, VkHash: c.VkHash,
			CallID: c.CallID, StartedAt: c.StartedAt, Finishing: c.Finishing,
		})
	}
	return out, nil
}

func findVKCreatorSessions(password, callID string) ([]vkCreatorSession, error) {
	all, err := listVKCreatorSessions()
	if err != nil {
		return nil, err
	}
	password = strings.TrimSpace(password)
	callID = strings.TrimSpace(callID)
	realPass := password
	if password != "" {
		if p, err := resolveUserPassword(password); err == nil {
			realPass = p
		}
	}
	var out []vkCreatorSession
	for _, s := range all {
		if callID != "" && s.CallID == callID {
			out = append(out, s)
		} else if callID == "" && password != "" &&
			(s.Password == realPass || s.Password == password || maskPassword(s.Password) == password) {
			out = append(out, s)
		}
	}
	return out, nil
}

func insertVKCreatorSession(s vkCreatorSession) error {
	if !panelDBEnabled() {
		return fmt.Errorf("база панели недоступна")
	}
	return paneldb.InsertVKCall(panelDB, paneldb.VKCall{
		CallID: s.CallID, Password: s.Password, JoinLink: s.JoinLink,
		VkHash: s.VkHash, StartedAt: s.StartedAt,
	})
}

func countVKCreatorSessionsForPassword(password string) (int, error) {
	sessions, err := findVKCreatorSessions(password, "")
	if err != nil {
		return 0, err
	}
	return len(sessions), nil
}

func markVKCreatorSessionFinishing(password, callID string) error {
	if !panelDBEnabled() {
		return fmt.Errorf("база панели недоступна")
	}
	callID = strings.TrimSpace(callID)
	password = strings.TrimSpace(password)
	if callID != "" {
		return paneldb.SetVKCallFinishing(panelDB, callID)
	}
	if password != "" {
		return paneldb.SetVKCallsFinishingByPassword(panelDB, password)
	}
	return fmt.Errorf("укажите call_id или профиль")
}

func deleteVKCreatorSessions(password, callID string) error {
	if !panelDBEnabled() {
		return fmt.Errorf("база панели недоступна")
	}
	callID = strings.TrimSpace(callID)
	password = strings.TrimSpace(password)
	if callID != "" {
		return paneldb.DeleteVKCall(panelDB, callID)
	}
	if password != "" {
		return paneldb.DeleteVKCallsByPassword(panelDB, password)
	}
	return fmt.Errorf("укажите call_id или профиль")
}

func dropVKCreatorSession(s vkCreatorSession) error {
	if !panelDBEnabled() {
		return fmt.Errorf("база панели недоступна")
	}
	callID := strings.TrimSpace(s.CallID)
	if callID == "" {
		return fmt.Errorf("call_id пуст")
	}
	if p := strings.TrimSpace(s.Password); p != "" {
		if realPass, err := resolveUserPassword(p); err == nil {
			p = realPass
		}
		if err := removeVKHashFromUser(p, s.VkHash); err != nil {
			return err
		}
	}
	return paneldb.DeleteVKCall(panelDB, callID)
}

func loadVKCookiesFromStore() ([]byte, error) {
	if !panelDBEnabled() {
		return nil, fmt.Errorf("база панели недоступна")
	}
	return paneldb.LoadVKCookiesJSON(panelDB)
}

func saveVKCookiesToStore(raw []byte) error {
	if !panelDBEnabled() {
		return fmt.Errorf("база панели недоступна")
	}
	return paneldb.SaveVKCookiesJSON(panelDB, raw)
}

func clearVKCookiesFromStore() error {
	if !panelDBEnabled() {
		return fmt.Errorf("база панели недоступна")
	}
	return paneldb.ClearVKCookies(panelDB)
}
