package gen

import (
	"encoding/json"
	"fmt"
	"sort"
)

// MethodResult is one entry of schema/method-results.json. Exactly one field
// is set: a single result variant, a list of possible variants (or "*" for any
// variant), or the acknowledging variant of a streaming method.
type MethodResult struct {
	Result  string   `json:"result,omitempty"`
	Results []string `json:"results,omitempty"`
	Stream  string   `json:"stream,omitempty"`
}

// MethodTable is the parsed schema/method-results.json.
type MethodTable struct {
	HerdrVersion string
	Protocol     uint32
	Methods      map[string]MethodResult
}

// ParseMethodTable reads the method result table.
func ParseMethodTable(data []byte) (*MethodTable, error) {
	var raw struct {
		HerdrVersion string                     `json:"herdr_version"`
		Protocol     uint32                     `json:"protocol"`
		Methods      map[string]json.RawMessage `json:"methods"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("method table: %w", err)
	}
	table := &MethodTable{
		HerdrVersion: raw.HerdrVersion,
		Protocol:     raw.Protocol,
		Methods:      make(map[string]MethodResult, len(raw.Methods)),
	}
	if len(raw.Methods) == 0 {
		return nil, fmt.Errorf("method table: no methods")
	}
	for _, name := range sortedKeys(raw.Methods) {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(raw.Methods[name], &entry); err != nil {
			return nil, fmt.Errorf("method table: %s: %w", name, err)
		}
		var result MethodResult
		var keys []string
		for _, key := range sortedKeys(entry) {
			keys = append(keys, key)
			var err error
			switch key {
			case "result":
				err = json.Unmarshal(entry[key], &result.Result)
			case "results":
				err = json.Unmarshal(entry[key], &result.Results)
			case "stream":
				err = json.Unmarshal(entry[key], &result.Stream)
			default:
				return nil, fmt.Errorf("method table: %s: unsupported key %q", name, key)
			}
			if err != nil {
				return nil, fmt.Errorf("method table: %s: %s: %w", name, key, err)
			}
		}
		if len(keys) != 1 {
			return nil, fmt.Errorf("method table: %s: expected exactly one of result, results, stream, got %v", name, keys)
		}
		if len(result.Results) == 0 && result.Result == "" && result.Stream == "" {
			return nil, fmt.Errorf("method table: %s: empty entry", name)
		}
		table.Methods[name] = result
	}
	return table, nil
}

// validate reports methods that the table and the schema do not agree on, and
// result names the schema does not define.
func (t *MethodTable) validate(methods []string, resultTags map[string]bool) error {
	known := make(map[string]bool, len(methods))
	for _, m := range methods {
		known[m] = true
	}
	var missing []string
	for _, m := range methods {
		if _, ok := t.Methods[m]; !ok {
			missing = append(missing, m)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("method table: no entry for %v", missing)
	}
	var unknown []string
	for name := range t.Methods {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("method table: unknown method %v", unknown)
	}
	for _, name := range sortedKeys(t.Methods) {
		entry := t.Methods[name]
		var tags []string
		switch {
		case entry.Result != "":
			tags = []string{entry.Result}
		case entry.Stream != "":
			tags = []string{entry.Stream}
		default:
			for _, tag := range entry.Results {
				if tag != "*" {
					tags = append(tags, tag)
				}
			}
		}
		for _, tag := range tags {
			if !resultTags[tag] {
				return fmt.Errorf("method table: %s: unknown result type %q", name, tag)
			}
		}
	}
	return nil
}
