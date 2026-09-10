package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func schemaWith(methods []string, defs map[string]string) *schemaDoc {
	doc := &schemaDoc{Protocol: 22}
	for _, name := range methods {
		var variant struct {
			Properties struct {
				Method struct {
					Const string `json:"const"`
				} `json:"method"`
			} `json:"properties"`
		}
		variant.Properties.Method.Const = name
		doc.Schemas.Request.OneOf = append(doc.Schemas.Request.OneOf, variant)
	}
	doc.Schemas.Request.Defs = map[string]json.RawMessage{}
	for name, shape := range defs {
		doc.Schemas.Request.Defs[name] = json.RawMessage(shape)
	}
	return doc
}

func differences(t *testing.T, before, after *schemaDoc) []string {
	t.Helper()
	r := &report{}
	compareSchemas(r, before, after)
	return r.differences
}

func TestCompareSchemasReportsMethodAndTypeDrift(t *testing.T) {
	before := schemaWith([]string{"ping", "pane.get"}, map[string]string{
		"PaneInfo":   `{"type":"object","properties":{"pane_id":{"type":"string"}}}`,
		"GoneParams": `{"type":"object"}`,
	})
	after := schemaWith([]string{"ping", "pane.peek"}, map[string]string{
		"PaneInfo":  `{"properties":{"pane_id":{"type":"integer"}},"type":"object"}`,
		"NewParams": `{"type":"object"}`,
	})
	after.Protocol = 23

	got := strings.Join(differences(t, before, after), "\n")
	for _, want := range []string{
		"protocol 22 in the snapshot, 23 in the installed binary",
		"method pane.peek is new",
		"method pane.get is gone",
		"type request.NewParams is new",
		"type request.GoneParams is gone",
		"type request.PaneInfo changed shape",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A definition that differs only in key order is the same shape.
func TestCompareSchemasIgnoresKeyOrder(t *testing.T) {
	before := schemaWith([]string{"ping"}, map[string]string{
		"PaneInfo": `{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}}}`,
	})
	after := schemaWith([]string{"ping"}, map[string]string{
		"PaneInfo": `{"properties":{"b":{"type":"string"},"a":{"type":"string"}},"type":"object"}`,
	})
	if got := differences(t, before, after); len(got) != 0 {
		t.Errorf("reported %v for an identical shape", got)
	}
}

func TestDecodeSchemaRejectsAMethodlessDocument(t *testing.T) {
	if _, err := decodeSchema([]byte(`{"protocol":22,"schemas":{}}`)); err == nil {
		t.Fatal("accepted a schema that declares no methods")
	}
}

func TestLoadGapsAcceptsAMissingFile(t *testing.T) {
	gaps, err := loadGaps(t.TempDir() + "/absent.json")
	if err != nil {
		t.Fatalf("loadGaps on a missing file: %v", err)
	}
	if len(gaps.ServerMethodsAbsentFromSchema) != 0 {
		t.Errorf("expected no gaps, got %v", gaps.ServerMethodsAbsentFromSchema)
	}
}

// The method list is only ever read out of an error message, so the parsing
// has to survive the surrounding prose.
func TestQuotedNamesParseOutOfAnErrorMessage(t *testing.T) {
	message := "invalid request: unknown variant `zzz.nope`, expected one of `ping`, `pane.get`, `plugin.pane.open` at line 1 column 46"
	var got []string
	for _, match := range quoted.FindAllStringSubmatch(message, -1) {
		got = append(got, match[1])
	}
	want := []string{"zzz.nope", "ping", "pane.get", "plugin.pane.open"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parsed %v, want %v", got, want)
	}
}
