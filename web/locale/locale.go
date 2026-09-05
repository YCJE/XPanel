// Package locale provides localization support for the XPanel web interface.
// The panel ships with Simplified Chinese as the single interface language.
package locale

import (
	"embed"
	"io/fs"
	"os"
	"strings"

	"github.com/YCJE/XPanel/logger"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

var (
	i18nBundle   *i18n.Bundle
	LocalizerWeb *i18n.Localizer
)

// I18nType represents the type of interface for localization.
type I18nType string

const (
	Web I18nType = "web" // Web interface type
)

// defaultLanguage is the single interface language of the panel.
const defaultLanguage = "zh-CN"

// InitLocalizer initializes the localization system with embedded translation files.
func InitLocalizer(i18nFS embed.FS, settingService interface{}) error {
	i18nBundle = i18n.NewBundle(language.MustParse(defaultLanguage))
	i18nBundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	if err := parseTranslationFiles(i18nFS, i18nBundle); err != nil {
		return err
	}

	LocalizerWeb = i18n.NewLocalizer(i18nBundle, defaultLanguage)
	return nil
}

// createTemplateData creates a template data map from parameters with optional separator.
func createTemplateData(params []string, separator ...string) map[string]any {
	var sep string = "=="
	if len(separator) > 0 {
		sep = separator[0]
	}

	templateData := make(map[string]any)
	for _, param := range params {
		parts := strings.SplitN(param, sep, 2)
		templateData[parts[0]] = parts[1]
	}

	return templateData
}

// I18n retrieves a localized message for the given key and type.
// Returns the localized message or an empty string if localization fails.
func I18n(i18nType I18nType, key string, params ...string) string {
	if i18nType != Web {
		logger.Errorf("Invalid type for I18n: %s", i18nType)
		return ""
	}

	templateData := createTemplateData(params)

	if LocalizerWeb == nil {
		// Fallback to key if localizer not ready; prevents nil panic on pages like sub
		return key
	}

	msg, err := LocalizerWeb.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: templateData,
	})
	if err != nil {
		logger.Errorf("Failed to localize message: %v", err)
		return ""
	}

	return msg
}

// LocalizerMiddleware returns a Gin middleware that provides the I18n
// function in the Gin context for template rendering.
func LocalizerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Ensure bundle is initialized so localization won't panic
		if i18nBundle == nil {
			i18nBundle = i18n.NewBundle(language.MustParse(defaultLanguage))
			i18nBundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
			// Try lazy-load from disk when running sub server without InitLocalizer
			if err := loadTranslationsFromDisk(i18nBundle); err != nil {
				logger.Warning("i18n lazy load failed:", err)
			}
			LocalizerWeb = i18n.NewLocalizer(i18nBundle, defaultLanguage)
		}

		c.Set("localizer", LocalizerWeb)
		c.Set("I18n", I18n)
		c.Next()
	}
}

// loadTranslationsFromDisk attempts to load translation files from "web/translation" using the local filesystem.
func loadTranslationsFromDisk(bundle *i18n.Bundle) error {
	root := os.DirFS("web")
	return fs.WalkDir(root, "translation", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		_, err = bundle.ParseMessageFileBytes(data, path)
		return err
	})
}

// parseTranslationFiles parses embedded translation files and adds them to the i18n bundle.
func parseTranslationFiles(i18nFS embed.FS, i18nBundle *i18n.Bundle) error {
	err := fs.WalkDir(i18nFS, "translation",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
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

	return nil
}
