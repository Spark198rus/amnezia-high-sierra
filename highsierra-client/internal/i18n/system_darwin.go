//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
)

// systemLanguage returns the first of the user's macOS languages, such as
// "ru-RU", or "" if it can't tell.
func systemLanguage() string {
	out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	// The output is a property list array: ( "ru-RU", "en-RU" ).
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.Trim(strings.TrimSpace(line), `(),"`)
		if line != "" {
			return line
		}
	}
	return ""
}
