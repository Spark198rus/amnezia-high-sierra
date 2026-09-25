// Package i18n shows awg-hs in Russian when that is the system language, and
// in English otherwise. English is the source language: a text is looked up
// by its English wording, and anything without a translation stays English.
package i18n

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// Lang is the language of this process's own output: "en" or "ru". The
// command-line tool sets it from Detect; the service answers each request in
// the language the request names.
var Lang = "en"

// Normalize maps a language tag such as "ru", "ru-RU" or "ru_RU.UTF-8" to
// "ru", and anything else to "en".
func Normalize(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "ru" || strings.HasPrefix(tag, "ru-") || strings.HasPrefix(tag, "ru_") || strings.HasPrefix(tag, "ru.") {
		return "ru"
	}
	return "en"
}

// Detect returns the user's language: the locale variables if one is set
// (Terminal sets LANG from the macOS language), or else the first of the
// system languages.
func Detect() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if tag := os.Getenv(v); tag != "" && tag != "C" && tag != "POSIX" && !strings.HasPrefix(tag, "C.") {
			return Normalize(tag)
		}
	}
	return Normalize(systemLanguage())
}

// T translates one of the command-line tool's texts into Lang.
func T(s string) string {
	return For(Lang, s)
}

// For translates s into lang.
func For(lang, s string) string {
	if Normalize(lang) == "ru" {
		if t, ok := ru[s]; ok {
			return t
		}
	}
	return s
}

// Message translates an error or warning into lang. Such messages are built
// from pieces ("line 3: PrivateKey: key must be 32 bytes of base64"), so it
// replaces the phrases it knows; the rest, such as errors from macOS tools,
// stays English.
func Message(lang, msg string) string {
	if Normalize(lang) != "ru" {
		return msg
	}
	msg = lineNumber.ReplaceAllString(msg, "строка $1: ")
	for _, p := range sortedPhrases {
		msg = strings.ReplaceAll(msg, p.en, p.ru)
	}
	return msg
}

var lineNumber = regexp.MustCompile(`\bline (\d+): `)

type phrase struct{ en, ru string }

// sortedPhrases has the longest phrases first, so a phrase inside a longer
// one never wins.
var sortedPhrases = func() []phrase {
	p := append([]phrase(nil), phrases...)
	sort.SliceStable(p, func(i, j int) bool { return len(p[i].en) > len(p[j].en) })
	return p
}()
