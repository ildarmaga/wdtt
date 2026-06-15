package panel

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

//go:embed web/translation/*
var i18nFS embed.FS

var (
	i18nBundle   *i18n.Bundle
	localizerWeb *i18n.Localizer
)

func initI18n() error {
	i18nBundle = i18n.NewBundle(language.MustParse("ru-RU"))
	i18nBundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	err := fs.WalkDir(i18nFS, "web/translation", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return err
		}
		data, err := i18nFS.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = i18nBundle.ParseMessageFileBytes(data, path)
		return err
	})
	if err != nil {
		return err
	}
	localizerWeb = i18n.NewLocalizer(i18nBundle, "ru-RU", "en-US")
	return nil
}

func i18nWeb(key string, params ...string) string {
	templateData := make(map[string]interface{})
	for _, param := range params {
		parts := strings.SplitN(param, "==", 2)
		if len(parts) == 2 {
			templateData[parts[0]] = parts[1]
		}
	}
	msg, err := localizerWeb.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: templateData,
	})
	if err != nil {
		return key
	}
	return msg
}
