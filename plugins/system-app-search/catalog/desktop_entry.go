package catalog

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const maxDesktopEntryBytes = 256 << 10

// Only metadata is parsed here. Exec field codes and quoting belong to GIO,
// never to a shell command assembled by the search provider.
func desktopMetadata(r io.Reader, locale, desktop string, lookup func(string) (string, error)) (metadata, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDesktopEntryBytes+1))
	if err != nil || len(data) > maxDesktopEntryBytes || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return metadata{}, errors.New("invalid desktop entry")
	}
	values := map[string]string{}
	active := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), maxDesktopEntryBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			active = line == "[Desktop Entry]"
			continue
		}
		if !active {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := values[key]; exists {
			return metadata{}, errors.New("duplicate desktop key")
		}
		values[key] = strings.TrimSpace(value)
	}
	if scanner.Err() != nil {
		return metadata{}, scanner.Err()
	}
	if values["Type"] != "Application" || values["Hidden"] == "true" || values["NoDisplay"] == "true" || (values["Exec"] == "" && values["DBusActivatable"] != "true") {
		return metadata{}, errors.New("desktop entry is not a visible application")
	}
	matches := func(list string) bool {
		for _, current := range strings.Split(desktop, ":") {
			for _, item := range strings.Split(list, ";") {
				if current != "" && item == current {
					return true
				}
			}
		}
		return false
	}
	if only := values["OnlyShowIn"]; only != "" && !matches(only) {
		return metadata{}, errors.New("desktop entry excluded")
	}
	if matches(values["NotShowIn"]) {
		return metadata{}, errors.New("desktop entry excluded")
	}
	if executable := values["TryExec"]; executable != "" {
		if _, err := lookup(executable); err != nil {
			return metadata{}, errors.New("desktop executable unavailable")
		}
	}
	name := values["Name"]
	for _, language := range desktopLocales(locale) {
		if localized := values["Name["+language+"]"]; localized != "" {
			name = localized
			break
		}
	}
	name = strings.NewReplacer(`\s`, " ", `\n`, "\n", `\t`, "\t", `\r`, "\r", `\\`, `\`).Replace(name)
	if strings.TrimSpace(name) == "" {
		return metadata{}, errors.New("desktop entry has no name")
	}
	return metadata{DisplayName: name, Name: values["Name"]}, nil
}

func desktopLocales(locale string) []string {
	base, modifier, _ := strings.Cut(locale, "@")
	base, _, _ = strings.Cut(base, ".")
	language, _, hasCountry := strings.Cut(base, "_")
	var result []string
	if modifier != "" {
		result = append(result, base+"@"+modifier)
	}
	result = append(result, base)
	if hasCountry {
		if modifier != "" {
			result = append(result, language+"@"+modifier)
		}
		result = append(result, language)
	}
	return result
}
