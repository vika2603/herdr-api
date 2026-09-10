package gen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Node is one JSON Schema node of the subset used by the Herdr API schema.
// Keywords outside that subset are rejected while parsing.
type Node struct {
	Path string

	// Always holds the value of a boolean schema (`true` accepts any value).
	Always *bool

	// Ref is the bare definition name a "$ref" points at.
	Ref string

	Types       []string
	Format      string
	Description string
	Default     json.RawMessage
	Enum        []string
	Const       *string

	Properties map[string]*Node
	Required   []string
	Items      *Node
	AddProps   *Node
	OneOf      []*Node
	AnyOf      []*Node
	Defs       map[string]*Node
}

// ignoredKeywords are validation keywords that constrain accepted values but
// never change the generated Go type. Every other unknown keyword is an error.
var ignoredKeywords = map[string]bool{
	"$schema":          true,
	"title":            true,
	"minimum":          true,
	"maximum":          true,
	"exclusiveMinimum": true,
	"exclusiveMaximum": true,
	"multipleOf":       true,
	"minLength":        true,
	"maxLength":        true,
	"pattern":          true,
	"minItems":         true,
	"maxItems":         true,
	"uniqueItems":      true,
	"minProperties":    true,
	"maxProperties":    true,
	"propertyNames":    true,
}

// Document is a parsed Herdr API schema file.
type Document struct {
	Protocol      uint32
	SchemaVersion int
	Sections      map[string]*Node
}

// SectionNames returns the schema section names in a stable order.
func (d *Document) SectionNames() []string {
	names := make([]string, 0, len(d.Sections))
	for name := range d.Sections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var refPattern = regexp.MustCompile(`^#/schemas/[a-z_]+/\$defs/([A-Za-z0-9_]+)$`)

// ParseSchema reads the schema document and validates that it only uses the
// supported keyword subset.
func ParseSchema(data []byte) (*Document, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	doc := &Document{Sections: map[string]*Node{}}
	for _, key := range sortedKeys(top) {
		switch key {
		case "$schema", "title":
		case "protocol":
			if err := json.Unmarshal(top[key], &doc.Protocol); err != nil {
				return nil, fmt.Errorf("schema: protocol: %w", err)
			}
		case "schema_version":
			if err := json.Unmarshal(top[key], &doc.SchemaVersion); err != nil {
				return nil, fmt.Errorf("schema: schema_version: %w", err)
			}
		case "schemas":
			var sections map[string]json.RawMessage
			if err := json.Unmarshal(top[key], &sections); err != nil {
				return nil, fmt.Errorf("schema: schemas: %w", err)
			}
			for _, name := range sortedKeys(sections) {
				node, err := parseNode("/"+name, sections[name])
				if err != nil {
					return nil, err
				}
				doc.Sections[name] = node
			}
		default:
			return nil, fmt.Errorf("schema: unsupported top-level key %q", key)
		}
	}
	if len(doc.Sections) == 0 {
		return nil, fmt.Errorf("schema: no sections")
	}
	return doc, nil
}

func parseNode(path string, raw json.RawMessage) (*Node, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "true" || trimmed == "false" {
		always := trimmed == "true"
		return &Node{Path: path, Always: &always}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	n := &Node{Path: path}
	for _, key := range sortedKeys(obj) {
		val := obj[key]
		var err error
		switch key {
		case "$ref":
			var ref string
			if err = json.Unmarshal(val, &ref); err != nil {
				break
			}
			m := refPattern.FindStringSubmatch(ref)
			if m == nil {
				return nil, fmt.Errorf("%s: unsupported $ref %q", path, ref)
			}
			n.Ref = m[1]
		case "type":
			n.Types, err = parseStringOrList(val)
		case "format":
			err = json.Unmarshal(val, &n.Format)
		case "description":
			err = json.Unmarshal(val, &n.Description)
		case "default":
			n.Default = append(json.RawMessage(nil), val...)
		case "enum":
			err = json.Unmarshal(val, &n.Enum)
			if err != nil {
				return nil, fmt.Errorf("%s: enum must be a list of strings: %w", path, err)
			}
		case "const":
			var s string
			if err = json.Unmarshal(val, &s); err != nil {
				return nil, fmt.Errorf("%s: const must be a string: %w", path, err)
			}
			n.Const = &s
		case "required":
			err = json.Unmarshal(val, &n.Required)
		case "properties":
			n.Properties, err = parseNodeMap(path+"/properties", val)
		case "$defs":
			n.Defs, err = parseNodeMap(path+"/$defs", val)
		case "items":
			n.Items, err = parseNode(path+"/items", val)
		case "additionalProperties":
			n.AddProps, err = parseNode(path+"/additionalProperties", val)
		case "oneOf":
			n.OneOf, err = parseNodeList(path+"/oneOf", val)
		case "anyOf":
			n.AnyOf, err = parseNodeList(path+"/anyOf", val)
		default:
			if ignoredKeywords[key] {
				continue
			}
			return nil, fmt.Errorf("%s: unsupported schema keyword %q", path, key)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, key, err)
		}
	}
	return n, nil
}

func parseNodeMap(path string, raw json.RawMessage) (map[string]*Node, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(map[string]*Node, len(m))
	for _, name := range sortedKeys(m) {
		node, err := parseNode(path+"/"+name, m[name])
		if err != nil {
			return nil, err
		}
		out[name] = node
	}
	return out, nil
}

func parseNodeList(path string, raw json.RawMessage) ([]*Node, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]*Node, 0, len(list))
	for i, item := range list {
		node, err := parseNode(fmt.Sprintf("%s[%d]", path, i), item)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, nil
}

func parseStringOrList(raw json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Nullable reports whether the node accepts JSON null.
func (n *Node) Nullable() bool {
	for _, t := range n.Types {
		if t == "null" {
			return true
		}
	}
	return false
}

// BaseType returns the single non-null JSON type of the node.
func (n *Node) BaseType() (string, error) {
	var base string
	for _, t := range n.Types {
		if t == "null" {
			continue
		}
		if base != "" {
			return "", fmt.Errorf("%s: unsupported type union %v", n.Path, n.Types)
		}
		base = t
	}
	return base, nil
}

// IsRequired reports whether the named property is required.
func (n *Node) IsRequired(name string) bool {
	for _, r := range n.Required {
		if r == name {
			return true
		}
	}
	return false
}

// PropertyNames returns the property names in a stable order.
func (n *Node) PropertyNames() []string {
	names := make([]string, 0, len(n.Properties))
	for name := range n.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// canonical renders the node so that two definitions of the same shape compare
// equal. $ref values are already reduced to bare definition names.
func canonical(n *Node) string {
	var b strings.Builder
	writeCanonical(&b, n)
	return b.String()
}

func writeCanonical(b *strings.Builder, n *Node) {
	if n == nil {
		b.WriteString("null")
		return
	}
	if n.Always != nil {
		fmt.Fprintf(b, "%t", *n.Always)
		return
	}
	b.WriteString("{")
	fmt.Fprintf(b, "ref=%q,", n.Ref)
	fmt.Fprintf(b, "types=%v,", n.Types)
	fmt.Fprintf(b, "format=%q,", n.Format)
	fmt.Fprintf(b, "enum=%v,", n.Enum)
	if n.Const != nil {
		fmt.Fprintf(b, "const=%q,", *n.Const)
	}
	if len(n.Default) > 0 {
		fmt.Fprintf(b, "default=%s,", string(n.Default))
	}
	required := append([]string(nil), n.Required...)
	sort.Strings(required)
	fmt.Fprintf(b, "required=%v,", required)
	b.WriteString("properties={")
	for _, name := range n.PropertyNames() {
		fmt.Fprintf(b, "%q:", name)
		writeCanonical(b, n.Properties[name])
		b.WriteString(",")
	}
	b.WriteString("},items=")
	writeCanonical(b, n.Items)
	b.WriteString(",addProps=")
	writeCanonical(b, n.AddProps)
	b.WriteString(",oneOf=[")
	for _, sub := range n.OneOf {
		writeCanonical(b, sub)
		b.WriteString(",")
	}
	b.WriteString("],anyOf=[")
	for _, sub := range n.AnyOf {
		writeCanonical(b, sub)
		b.WriteString(",")
	}
	b.WriteString("]}")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
