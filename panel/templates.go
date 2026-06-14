package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed web/html/*
var htmlFS embed.FS

//go:embed web/assets/*
var assetsFS embed.FS

var (
	htmlTemplates *template.Template
	startTime     = time.Now()
	assetsVer     string
)

func initAssetsVer() {
	data, err := assetsFS.ReadFile("web/assets/css/custom.min.css")
	if err != nil {
		assetsVer = "0"
		return
	}
	sum := sha256.Sum256(data)
	assetsVer = hex.EncodeToString(sum[:4])
}

func initTemplates() error {
	funcMap := template.FuncMap{
		"i18n": func(key string, params ...string) string {
			return i18nWeb(key, params...)
		},
	}
	t := template.New("").Funcs(funcMap)
	err := fs.WalkDir(htmlFS, "web/html", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}
		newT, parseErr := t.ParseFS(htmlFS, path)
		if parseErr != nil {
			return parseErr
		}
		t = newT
		return nil
	})
	if err != nil {
		return err
	}
	htmlTemplates = t
	return nil
}

type pageData map[string]interface{}

func (a *App) refreshCSRFForRequest(w http.ResponseWriter, r *http.Request) {
	if sess := a.parseSession(r); sess != nil {
		a.setCSRFCookie(w, sess)
	}
}

func (a *App) csrfTokenFromRequest(r *http.Request) string {
	if sess := a.parseSession(r); sess != nil {
		return a.csrfTokenForSession(sess)
	}
	return ""
}

func (a *App) renderHTML(w http.ResponseWriter, r *http.Request, name, title string, extra pageData) {
	if extra == nil {
		extra = pageData{}
	}
	a.refreshCSRFForRequest(w, r)
	if token := a.csrfTokenFromRequest(r); token != "" {
		extra["csrf_token"] = token
	}
	extra["title"] = title
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Header.Get("X-Real-IP")
	}
	if host == "" {
		var err error
		host, _, err = net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
	}
	extra["host"] = host
	extra["request_uri"] = r.RequestURI
	extra["base_path"] = a.cfg.basePath()
	extra["cur_ver"] = panelVersion
	extra["assets_ver"] = assetsVer

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := htmlTemplates.ExecuteTemplate(w, name, extra); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}

type wrapAssetsFS struct {
	embed.FS
}

func (f *wrapAssetsFS) Open(name string) (fs.File, error) {
	return f.FS.Open("web/assets/" + name)
}

func assetsHandler() http.Handler {
	sub, _ := fs.Sub(assetsFS, "web/assets")
	return http.FileServer(http.FS(sub))
}
