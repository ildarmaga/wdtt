package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func wdttAdminBaseURL() string {
	cfg, err := loadWdttInbound()
	addr := "127.0.0.1:2861"
	if err == nil && strings.TrimSpace(cfg.AdminAddr) != "" {
		addr = strings.TrimSpace(cfg.AdminAddr)
	}
	return "http://" + addr
}

func wdttAdminPost(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, wdttAdminBaseURL()+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg, err := loadPanelConfig(); err == nil && cfg != nil {
		if key := strings.TrimSpace(cfg.SessionKey); key != "" {
			req.Header.Set("X-WDTT-Admin-Token", key)
		}
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("admin API: unauthorized")
	}
	var out struct {
		OK  bool   `json:"ok"`
		Msg string `json:"msg"`
	}
	if json.Unmarshal(raw, &out) == nil && !out.OK {
		if out.Msg != "" {
			return fmt.Errorf("%s", out.Msg)
		}
		return fmt.Errorf("admin API error")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("admin API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func serverApplyUserUpdate(oldPassword, newPassword string, req userAPIReq, manageDevices bool) error {
	body := panelUserUpdateReq{
		OldPassword:    oldPassword,
		Password:       newPassword,
		DeviceID:       req.DeviceID,
		DeviceIDs:      req.DeviceIDs,
		MaxDevices:     req.MaxDevices,
		Comment:        req.Comment,
		ExpiresAt:      req.ExpiresAt,
		TotalGB:        req.TotalGB,
		MaxDownMBps:    req.MaxDownMBps,
		MaxUpMBps:      req.MaxUpMBps,
		Active:         req.Active,
		Ports:          req.Ports,
		DtlsPort:       req.DtlsPort,
		WgPort:         req.WgPort,
		ClientPort:     req.ClientPort,
		UseCustomPorts: req.UseCustomPorts,
		VkHash:         req.VkHash,
	}
	if !manageDevices {
		body.DeviceIDs = nil
	}
	return wdttAdminPost("/admin/users/update", body)
}

func serverDeleteUser(pass string) error {
	return wdttAdminPost("/admin/users/delete", map[string]string{"password": pass})
}

type panelUserUpdateReq struct {
	OldPassword    string    `json:"old_password"`
	Password       string    `json:"password"`
	DeviceID       string    `json:"device_id"`
	DeviceIDs      *[]string `json:"device_ids"`
	MaxDevices     int       `json:"max_devices"`
	Comment        string    `json:"comment"`
	ExpiresAt      int64     `json:"expires_at"`
	TotalGB        float64   `json:"total_gb"`
	MaxDownMBps    float64   `json:"max_down_mbps"`
	MaxUpMBps      float64   `json:"max_up_mbps"`
	Active         *bool     `json:"active"`
	Ports          string    `json:"ports"`
	DtlsPort       int       `json:"dtls_port"`
	WgPort         int       `json:"wg_port"`
	ClientPort     int       `json:"client_port"`
	UseCustomPorts bool      `json:"use_custom_ports"`
	VkHash         string    `json:"vk_hash"`
}
