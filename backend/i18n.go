package main

// i18n.go: the interface language (Portuguese or English).
//
// The language is the "language" setting in bancada.yaml. Messages are
// written in place with both versions, so it's easy to see what each says:
//
//	errors.New(tr("arquivo não encontrado", "file not found"))

import "sync/atomic"

var englishUI atomic.Bool // false = Portuguese (the default)

// setLanguage is called every time bancada.yaml is read.
func setLanguage(lang string) { englishUI.Store(lang == "en") }

// tr returns the Portuguese or the English text, depending on the setting.
func tr(pt, en string) string {
	if englishUI.Load() {
		return en
	}
	return pt
}

func validLanguage(lang string) bool { return lang == "pt-BR" || lang == "en" }

// currentLanguage is sent to the Python service so its reasons match the UI.
func currentLanguage() string {
	if englishUI.Load() {
		return "en"
	}
	return "pt-BR"
}
