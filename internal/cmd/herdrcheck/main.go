// Command herdrcheck reports how far the installed herdr has moved from the
// schema snapshot this module generates from.
//
// Three kinds of drift matter and only the first is visible to the compiler
// after a regeneration, so this command looks for all three:
//
//   - Schema drift: methods, definitions or the protocol number differ between
//     the installed binary's schema and schema/herdr-api.schema.json.
//   - Method drift: the running server accepts a method the schema does not
//     declare, which no generated wrapper can reach. The server lists every
//     method it accepts in the error it returns for an unknown one.
//   - Version drift: the running server reports a version or protocol other
//     than the one the snapshot was taken from.
//
// It changes nothing and calls only ping. A non-zero exit means drift was
// found; -q prints only the differences.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vika2603/herdr-client/herdr"
)

func main() {
	var (
		schemaPath  = flag.String("schema", "schema/herdr-api.schema.json", "path to the schema snapshot")
		methodsPath = flag.String("methods", "schema/method-results.json", "path to the method result table")
		gapsPath    = flag.String("gaps", "schema/known-gaps.json", "path to the accepted-gaps list")
		binary      = flag.String("herdr", "herdr", "herdr binary to ask for the current schema")
		quiet       = flag.Bool("q", false, "print differences only")
	)
	flag.Parse()

	report, err := run(*schemaPath, *methodsPath, *gapsPath, *binary)
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdrcheck:", err)
		os.Exit(2)
	}
	fmt.Print(report.text(*quiet))
	if len(report.differences) > 0 {
		os.Exit(1)
	}
}

type report struct {
	lines       []string
	differences []string
}

func (r *report) note(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *report) differ(format string, args ...any) {
	r.differences = append(r.differences, fmt.Sprintf(format, args...))
}

// text renders the report. Differences are always shown; quiet drops the
// descriptive lines that precede them.
func (r *report) text(quiet bool) string {
	var b strings.Builder
	if !quiet {
		for _, line := range r.lines {
			b.WriteString(line + "\n")
		}
	}
	if len(r.differences) == 0 {
		if !quiet {
			b.WriteString("no drift\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "\n%d difference(s):\n", len(r.differences))
	for _, line := range r.differences {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\nAfter reviewing these, run `just schema-update && just gen && just check`,\n")
	b.WriteString("add any new method to schema/method-results.json, and walk the upgrade\n")
	b.WriteString("checklist in docs/design.md for the behaviour the schema does not describe.\n")
	return b.String()
}

func run(schemaPath, methodsPath, gapsPath, binary string) (*report, error) {
	r := &report{}

	snapshot, err := loadSchema(schemaPath)
	if err != nil {
		return nil, err
	}
	table, err := loadTable(methodsPath)
	if err != nil {
		return nil, err
	}
	r.note("snapshot: herdr %s, protocol %d, %d methods", table.HerdrVersion, snapshot.Protocol, len(snapshot.methods()))

	current, err := currentSchema(binary)
	if err != nil {
		r.note("installed binary: %v", err)
	} else {
		r.note("installed binary: protocol %d, %d methods", current.Protocol, len(current.methods()))
		compareSchemas(r, snapshot, current)
	}

	gaps, err := loadGaps(gapsPath)
	if err != nil {
		return nil, err
	}
	if err := compareServer(r, snapshot, table, gaps); err != nil {
		r.note("running server: %v", err)
	}
	return r, nil
}

type schemaDoc struct {
	Protocol uint32 `json:"protocol"`
	Schemas  struct {
		Request struct {
			OneOf []struct {
				Properties struct {
					Method struct {
						Const string `json:"const"`
					} `json:"method"`
				} `json:"properties"`
			} `json:"oneOf"`
			Defs map[string]json.RawMessage `json:"$defs"`
		} `json:"request"`
		SuccessResponse struct {
			Defs map[string]json.RawMessage `json:"$defs"`
		} `json:"success_response"`
		Event struct {
			Defs map[string]json.RawMessage `json:"$defs"`
		} `json:"event"`
	} `json:"schemas"`
}

func (d *schemaDoc) methods() map[string]bool {
	set := make(map[string]bool, len(d.Schemas.Request.OneOf))
	for _, variant := range d.Schemas.Request.OneOf {
		if name := variant.Properties.Method.Const; name != "" {
			set[name] = true
		}
	}
	return set
}

// definitions returns every named type across the sections that carry them,
// with the JSON of its shape, so a changed field is visible and not only a
// renamed type.
func (d *schemaDoc) definitions() map[string]string {
	all := make(map[string]string)
	for section, defs := range map[string]map[string]json.RawMessage{
		"request":          d.Schemas.Request.Defs,
		"success_response": d.Schemas.SuccessResponse.Defs,
		"event":            d.Schemas.Event.Defs,
	} {
		for name, raw := range defs {
			all[section+"."+name] = canonical(raw)
		}
	}
	return all
}

// canonical re-encodes a definition so that key order cannot make two equal
// shapes look different.
func canonical(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

// knownGaps lists differences that are understood and accepted, so that a
// standing gap does not mask new drift by failing every run.
type knownGaps struct {
	ServerMethodsAbsentFromSchema map[string]string `json:"server_methods_absent_from_schema"`
}

func loadGaps(path string) (*knownGaps, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &knownGaps{}, nil
	}
	if err != nil {
		return nil, err
	}
	var gaps knownGaps
	if err := json.Unmarshal(data, &gaps); err != nil {
		return nil, fmt.Errorf("decode known gaps: %w", err)
	}
	return &gaps, nil
}

type resultTable struct {
	HerdrVersion string                     `json:"herdr_version"`
	Methods      map[string]json.RawMessage `json:"methods"`
}

func loadSchema(path string) (*schemaDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeSchema(data)
}

func decodeSchema(data []byte) (*schemaDoc, error) {
	var doc schemaDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	if len(doc.Schemas.Request.OneOf) == 0 {
		return nil, errors.New("schema declares no methods")
	}
	return &doc, nil
}

func loadTable(path string) (*resultTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var table resultTable
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, fmt.Errorf("decode method table: %w", err)
	}
	return &table, nil
}

func currentSchema(binary string) (*schemaDoc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "api", "schema", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("run %s api schema --json: %w", binary, err)
	}
	return decodeSchema(out)
}

func compareSchemas(r *report, snapshot, current *schemaDoc) {
	if snapshot.Protocol != current.Protocol {
		r.differ("protocol %d in the snapshot, %d in the installed binary", snapshot.Protocol, current.Protocol)
	}
	for _, name := range sortedDiff(current.methods(), snapshot.methods()) {
		r.differ("method %s is new in the installed binary", name)
	}
	for _, name := range sortedDiff(snapshot.methods(), current.methods()) {
		r.differ("method %s is gone from the installed binary", name)
	}

	before, after := snapshot.definitions(), current.definitions()
	for _, name := range sortedMissing(after, before) {
		r.differ("type %s is new", name)
	}
	for _, name := range sortedMissing(before, after) {
		r.differ("type %s is gone", name)
	}
	var changed []string
	for name, shape := range before {
		if other, ok := after[name]; ok && other != shape {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	for _, name := range changed {
		r.differ("type %s changed shape", name)
	}
}

// compareServer asks a running server for its version and for the methods it
// accepts. The method list is only available in the error returned for an
// unknown method, which is why this is a probe rather than a query.
func compareServer(r *report, snapshot *schemaDoc, table *resultTable, gaps *knownGaps) error {
	client, err := herdr.NewFromEnv()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pong, err := client.Ping(ctx)
	if err != nil {
		return err
	}
	r.note("running server: herdr %s, protocol %d", pong.Version, pong.Protocol)
	if pong.Version != table.HerdrVersion {
		r.differ("server reports herdr %s, the snapshot was taken from %s", pong.Version, table.HerdrVersion)
	}
	if pong.Protocol != snapshot.Protocol {
		r.differ("server reports protocol %d, the snapshot declares %d", pong.Protocol, snapshot.Protocol)
	}

	accepted, err := acceptedMethods(ctx, client)
	if err != nil {
		return err
	}
	r.note("running server accepts %d methods", len(accepted))
	for _, name := range sortedDiff(accepted, snapshot.methods()) {
		if _, known := gaps.ServerMethodsAbsentFromSchema[name]; known {
			r.note("known gap: %s is accepted by the server and absent from the schema", name)
			continue
		}
		r.differ("server accepts %s but the schema does not declare it, so no wrapper reaches it", name)
	}
	for name := range gaps.ServerMethodsAbsentFromSchema {
		if !accepted[name] {
			r.differ("known gap %s is no longer accepted by the server; remove it from the gaps list", name)
		}
	}
	return nil
}

// unknownMethod is a name no herdr version will define. The server answers
// with invalid_request and lists every method it does accept.
const unknownMethod = "herdrcheck.unknown.method"

var quoted = regexp.MustCompile("`([a-z_][a-z0-9_.]*)`")

func acceptedMethods(ctx context.Context, client *herdr.Client) (map[string]bool, error) {
	_, err := client.CallRaw(ctx, unknownMethod, nil)
	var apiErr *herdr.Error
	if !errors.As(err, &apiErr) {
		return nil, fmt.Errorf("probing for the method list: %w", err)
	}
	set := make(map[string]bool)
	for _, match := range quoted.FindAllStringSubmatch(apiErr.Message, -1) {
		if name := match[1]; strings.Contains(name, ".") || name == "ping" {
			set[name] = true
		}
	}
	delete(set, unknownMethod)
	if len(set) == 0 {
		return nil, fmt.Errorf("the server did not list its methods: %s", apiErr.Message)
	}
	return set, nil
}

// sortedDiff returns the sorted keys of from that are absent from in.
func sortedDiff(from, in map[string]bool) []string {
	var missing []string
	for name := range from {
		if !in[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func sortedMissing(from, in map[string]string) []string {
	var missing []string
	for name := range from {
		if _, ok := in[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}
