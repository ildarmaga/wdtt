package paneldb

import (
	"database/sql"
	"fmt"
)

const (
	DefaultPanelPort = 2860
	DefaultSubPort   = 2096
)

// PanelConfig — строка panel_config (id=1).
type PanelConfig struct {
	Username      string
	PasswordHash  string
	Port          int
	WebBasePath   string
	SessionKey    string
	WebListen     string
	WebDomain     string
	WebCertFile   string
	WebKeyFile    string
	SessionMaxAge int
	PageSize      int
	RemarkModel   string
	BlockPing     bool
	SubEnable     bool
	SubListen     string
	SubPort       int
	SubPath       string
	SubDomain     string
	SubCertFile   string
	SubKeyFile    string
	SubEncrypt    bool
	SubUpdates    int
	SubTitle      string
	SubSupportURL string
	SubProfileURL string
	SubAnnounce   string
	SubURI        string
	SubShowInfo   bool
}

// HasPanelConfig — есть ли строка в panel_config.
func HasPanelConfig(db *sql.DB) (bool, error) {
	n, err := tableCount(db, "panel_config")
	return n > 0, err
}

// LoadPanelConfig читает panel_config WHERE id = 1.
func LoadPanelConfig(db *sql.DB) (*PanelConfig, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	var cfg PanelConfig
	var blockPing, subEnable, subEncrypt, subShowInfo int
	err := db.QueryRow(`SELECT username, password_hash, port, web_base_path, session_key,
		web_listen, web_domain, web_cert_file, web_key_file, session_max_age, page_size, remark_model, block_ping,
		sub_enable, sub_listen, sub_port, sub_path, sub_domain, sub_cert_file, sub_key_file,
		sub_encrypt, sub_updates, sub_title, sub_support_url, sub_profile_url, sub_announce, sub_uri, sub_show_info
		FROM panel_config WHERE id = 1`).Scan(
		&cfg.Username, &cfg.PasswordHash, &cfg.Port, &cfg.WebBasePath, &cfg.SessionKey,
		&cfg.WebListen, &cfg.WebDomain, &cfg.WebCertFile, &cfg.WebKeyFile,
		&cfg.SessionMaxAge, &cfg.PageSize, &cfg.RemarkModel, &blockPing,
		&subEnable, &cfg.SubListen, &cfg.SubPort, &cfg.SubPath, &cfg.SubDomain, &cfg.SubCertFile, &cfg.SubKeyFile,
		&subEncrypt, &cfg.SubUpdates, &cfg.SubTitle, &cfg.SubSupportURL, &cfg.SubProfileURL, &cfg.SubAnnounce, &cfg.SubURI, &subShowInfo,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	cfg.BlockPing = blockPing != 0
	cfg.SubEnable = subEnable != 0
	cfg.SubEncrypt = subEncrypt != 0
	cfg.SubShowInfo = subShowInfo != 0
	return &cfg, nil
}

// SavePanelConfig upsert panel_config (id=1).
func SavePanelConfig(db *sql.DB, cfg *PanelConfig) error {
	if db == nil || cfg == nil {
		return fmt.Errorf("nil db or config")
	}
	bp := 0
	if cfg.BlockPing {
		bp = 1
	}
	se := 0
	if cfg.SubEnable {
		se = 1
	}
	senc := 0
	if cfg.SubEncrypt {
		senc = 1
	}
	sshow := 0
	if cfg.SubShowInfo {
		sshow = 1
	}
	_, err := db.Exec(`INSERT INTO panel_config (
		id, username, password_hash, port, web_base_path, session_key,
		web_listen, web_domain, web_cert_file, web_key_file,
		session_max_age, page_size, remark_model, block_ping,
		sub_enable, sub_listen, sub_port, sub_path, sub_domain, sub_cert_file, sub_key_file,
		sub_encrypt, sub_updates, sub_title, sub_support_url, sub_profile_url, sub_announce, sub_uri, sub_show_info
	) VALUES (1,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		username=excluded.username, password_hash=excluded.password_hash, port=excluded.port,
		web_base_path=excluded.web_base_path, session_key=excluded.session_key,
		web_listen=excluded.web_listen, web_domain=excluded.web_domain,
		web_cert_file=excluded.web_cert_file, web_key_file=excluded.web_key_file,
		session_max_age=excluded.session_max_age, page_size=excluded.page_size,
		remark_model=excluded.remark_model, block_ping=excluded.block_ping,
		sub_enable=excluded.sub_enable, sub_listen=excluded.sub_listen, sub_port=excluded.sub_port,
		sub_path=excluded.sub_path, sub_domain=excluded.sub_domain,
		sub_cert_file=excluded.sub_cert_file, sub_key_file=excluded.sub_key_file,
		sub_encrypt=excluded.sub_encrypt, sub_updates=excluded.sub_updates,
		sub_title=excluded.sub_title, sub_support_url=excluded.sub_support_url,
		sub_profile_url=excluded.sub_profile_url, sub_announce=excluded.sub_announce,
		sub_uri=excluded.sub_uri, sub_show_info=excluded.sub_show_info`,
		cfg.Username, cfg.PasswordHash, cfg.Port, cfg.WebBasePath, cfg.SessionKey,
		cfg.WebListen, cfg.WebDomain, cfg.WebCertFile, cfg.WebKeyFile,
		cfg.SessionMaxAge, cfg.PageSize, cfg.RemarkModel, bp,
		se, cfg.SubListen, cfg.SubPort, cfg.SubPath, cfg.SubDomain, cfg.SubCertFile, cfg.SubKeyFile,
		senc, cfg.SubUpdates, cfg.SubTitle, cfg.SubSupportURL, cfg.SubProfileURL, cfg.SubAnnounce, cfg.SubURI, sshow,
	)
	return err
}

// LoadPanelServicePorts — port и sub_port для wdtt-server (iptables / health).
func LoadPanelServicePorts(db *sql.DB) (panelPort, subPort int, ok bool, err error) {
	if db == nil {
		return 0, 0, false, fmt.Errorf("nil db")
	}
	var p, s sql.NullInt64
	err = db.QueryRow(`SELECT port, sub_port FROM panel_config WHERE id = 1`).Scan(&p, &s)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	panelPort = DefaultPanelPort
	subPort = DefaultSubPort
	if p.Valid && p.Int64 > 0 {
		panelPort = int(p.Int64)
	}
	if s.Valid && s.Int64 > 0 {
		subPort = int(s.Int64)
	}
	return panelPort, subPort, true, nil
}
