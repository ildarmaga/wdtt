package panel

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultWdttGithubUser = "ildarmaga"

type wdttGitHubRelease struct {
	TagName string `json:"tag_name"`
}

func wdttGithubUser() string {
	if u := strings.TrimSpace(os.Getenv("WDTT_GITHUB_USER")); u != "" {
		return u
	}
	return defaultWdttGithubUser
}

func panelReleaseBinName() string {
	switch runtime.GOARCH {
	case "arm64":
		return "wdtt-panel-linux-arm64"
	default:
		return "wdtt-panel-linux-amd64"
	}
}

func panelInstallPath() string {
	if p := strings.TrimSpace(os.Getenv("WDTT_PANEL_BIN")); p != "" {
		return p
	}
	return "/usr/local/bin/wdtt-panel"
}

func fetchWdttPanelReleases() ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/wdtt/releases?per_page=20", wdttGithubUser())
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}
	var releases []wdttGitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(releases))
	for _, rel := range releases {
		if rel.TagName != "" {
			tags = append(tags, rel.TagName)
		}
	}
	return filterReleaseTagsSinceDB(tags), nil
}

func installPanelVersion(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("версия не указана")
	}
	binName := panelReleaseBinName()
	url := fmt.Sprintf("https://github.com/%s/wdtt/releases/download/%s/%s", wdttGithubUser(), tag, binName)
	dest := panelInstallPath()
	tmp := filepath.Join(os.TempDir(), binName+".download")
	if err := downloadFile(url, tmp); err != nil {
		return fmt.Errorf("скачивание %s: %w", tag, err)
	}
	defer os.Remove(tmp)
	if err := os.Chmod(tmp, 0755); err != nil {
		return err
	}
	backup := dest + ".bak." + fmt.Sprintf("%d", time.Now().Unix())
	if _, err := os.Stat(dest); err == nil {
		_ = os.Rename(dest, backup)
	}
	if err := os.Rename(tmp, dest); err != nil {
		in, err := os.Open(tmp)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		if _, err := out.ReadFrom(in); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	log.Printf("[PANEL] updated to %s (%s)", tag, dest)
	go func() {
		time.Sleep(800 * time.Millisecond)
		if err := restartPanelService(); err != nil {
			log.Printf("[PANEL] self-restart after update: %v", err)
		}
	}()
	return nil
}
