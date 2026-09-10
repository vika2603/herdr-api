package gen

import "strings"

// initialisms are the words that are upper-cased as a whole when a snake_case
// name is converted to a Go identifier.
var initialisms = map[string]string{
	"ansi": "ANSI",
	"api":  "API",
	"cli":  "CLI",
	"id":   "ID",
	"json": "JSON",
	"ok":   "OK",
	"os":   "OS",
	"pid":  "PID",
	"tty":  "TTY",
	"ttl":  "TTL",
	"ui":   "UI",
	"url":  "URL",
}

// splitWords splits a wire name on the word boundaries the schema uses.
func splitWords(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	return fields
}

// pascal converts a wire name such as pane_id or pane.output_matched to a Go
// identifier such as PaneID or PaneOutputMatched.
func pascal(s string) string {
	var b strings.Builder
	for _, word := range splitWords(s) {
		if up, ok := initialisms[strings.ToLower(word)]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(word[:1]))
		b.WriteString(word[1:])
	}
	return b.String()
}

// enumConstName is the constant name for one value of a string enum.
func enumConstName(typeName, value string) string {
	return typeName + pascal(value)
}

// dotName converts an EventKind value to the dotted event name, matching
// EventKind::dot_name in herdr.
func dotName(kind string) string {
	return strings.Replace(kind, "_", ".", 1)
}
