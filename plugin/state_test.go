package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type settings struct {
	Theme string `json:"theme"`
	Limit int    `json:"limit"`
}

func TestStatePathAndConfigPath(t *testing.T) {
	env := &Env{StateDir: filepath.Join("/state", "example"), ConfigDir: "/config/example"}

	if got, want := env.StatePath(), filepath.Join("/state", "example"); got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
	if got, want := env.StatePath("log.jsonl"), filepath.Join("/state", "example", "log.jsonl"); got != want {
		t.Errorf("StatePath(name) = %q, want %q", got, want)
	}
	if got, want := env.StatePath("runs", "last.json"), filepath.Join("/state", "example", "runs", "last.json"); got != want {
		t.Errorf("StatePath(dir, name) = %q, want %q", got, want)
	}
	if got, want := env.ConfigPath("config.toml"), filepath.Join("/config", "example", "config.toml"); got != want {
		t.Errorf("ConfigPath(name) = %q, want %q", got, want)
	}
}

func TestStatePathRejectsWhatLeavesTheDirectory(t *testing.T) {
	env := &Env{StateDir: filepath.Join("/state", "example"), ConfigDir: filepath.Join("/config", "example")}

	names := [][]string{
		{".."},
		{"..", "other", "log"},
		{"runs", "..", "..", "other"},
		{""},
		{"a", ""},
		{filepath.Join(string(filepath.Separator), "etc", "passwd")},
	}
	for _, name := range names {
		if got := env.StatePath(name...); got != "" {
			t.Errorf("StatePath(%q) = %q, want the empty string", name, got)
		}
		if got := env.ConfigPath(name...); got != "" {
			t.Errorf("ConfigPath(%q) = %q, want the empty string", name, got)
		}
	}

	// A name that climbs and comes back stays inside and is accepted.
	if got, want := env.StatePath("runs", "..", "log"), filepath.Join("/state", "example", "log"); got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}

func TestStatePathWithoutADirectory(t *testing.T) {
	env := &Env{}
	if got := env.StatePath("log"); got != "" {
		t.Errorf("StatePath() = %q, want the empty string", got)
	}
	if got := env.ConfigPath("config.toml"); got != "" {
		t.Errorf("ConfigPath() = %q, want the empty string", got)
	}
	if got := env.StatePath(); got != "" {
		t.Errorf("StatePath() = %q, want the empty string", got)
	}
}

func TestStateHelpersReportAMissingDirectory(t *testing.T) {
	env := &Env{}

	if _, err := env.ReadState("log"); !errors.Is(err, ErrNoStateDir) {
		t.Errorf("ReadState() error = %v, want ErrNoStateDir", err)
	}
	if err := env.ReadStateJSON("state.json", &settings{}); !errors.Is(err, ErrNoStateDir) {
		t.Errorf("ReadStateJSON() error = %v, want ErrNoStateDir", err)
	}
	if err := env.WriteStateJSON("state.json", settings{}); !errors.Is(err, ErrNoStateDir) {
		t.Errorf("WriteStateJSON() error = %v, want ErrNoStateDir", err)
	}
	if err := env.AppendStateJSONL("log.jsonl", settings{}); !errors.Is(err, ErrNoStateDir) {
		t.Errorf("AppendStateJSONL() error = %v, want ErrNoStateDir", err)
	}
}

func TestStateHelpersRejectAnEscapingName(t *testing.T) {
	env := &Env{StateDir: t.TempDir()}
	name := filepath.Join("..", "escaped.json")

	if _, err := env.ReadState(name); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Errorf("ReadState() error = %v, want it to name the constraint", err)
	}
	if err := env.WriteStateJSON(name, settings{}); err == nil {
		t.Error("WriteStateJSON() error = nil, want one")
	}
	if err := env.AppendStateJSONL(name, settings{}); err == nil {
		t.Error("AppendStateJSONL() error = nil, want one")
	}
	if _, err := os.Stat(filepath.Join(env.StateDir, name)); err == nil {
		t.Error("a file was written outside the state directory")
	}
}

func TestReadStateJSONAbsentFile(t *testing.T) {
	env := &Env{StateDir: filepath.Join(t.TempDir(), "state")}

	got := settings{Theme: "dark", Limit: 10}
	if err := env.ReadStateJSON("state.json", &got); err != nil {
		t.Fatalf("ReadStateJSON() error = %v, want nil for an absent file", err)
	}
	if (got != settings{Theme: "dark", Limit: 10}) {
		t.Errorf("value = %+v, want the defaults it was called with", got)
	}

	data, err := env.ReadState("state.json")
	if err != nil || data != nil {
		t.Errorf("ReadState() = %q, %v, want nil, nil", data, err)
	}
}

func TestWriteStateJSONRoundTrip(t *testing.T) {
	env := &Env{StateDir: filepath.Join(t.TempDir(), "state", "nested")}

	if err := env.WriteStateJSON("state.json", settings{Theme: "dark", Limit: 3}); err != nil {
		t.Fatalf("WriteStateJSON() error = %v", err)
	}
	var got settings
	if err := env.ReadStateJSON("state.json", &got); err != nil {
		t.Fatalf("ReadStateJSON() error = %v", err)
	}
	if want := (settings{Theme: "dark", Limit: 3}); got != want {
		t.Errorf("value = %+v, want %+v", got, want)
	}

	if err := env.WriteStateJSON("state.json", settings{Theme: "light", Limit: 4}); err != nil {
		t.Fatalf("WriteStateJSON() error = %v", err)
	}
	if err := env.ReadStateJSON("state.json", &got); err != nil {
		t.Fatalf("ReadStateJSON() error = %v", err)
	}
	if want := (settings{Theme: "light", Limit: 4}); got != want {
		t.Errorf("value after rewrite = %+v, want %+v", got, want)
	}

	// The rename leaves the directory holding the target only.
	entries, err := os.ReadDir(env.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Errorf("state directory = %v, want state.json alone", entries)
	}
}

// A write that fails must leave the previous contents readable, which is the
// reason the helper renames rather than truncating.
func TestWriteStateJSONKeepsThePreviousFileWhenEncodingFails(t *testing.T) {
	env := &Env{StateDir: t.TempDir()}
	if err := env.WriteStateJSON("state.json", settings{Theme: "dark"}); err != nil {
		t.Fatal(err)
	}

	if err := env.WriteStateJSON("state.json", make(chan int)); err == nil {
		t.Fatal("WriteStateJSON() error = nil, want an encoding error")
	}

	var got settings
	if err := env.ReadStateJSON("state.json", &got); err != nil {
		t.Fatalf("ReadStateJSON() error = %v", err)
	}
	if got.Theme != "dark" {
		t.Errorf("value = %+v, want the previous contents", got)
	}
	entries, err := os.ReadDir(env.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("state directory = %v, want no temporary file left behind", entries)
	}
}

func TestReadStateJSONReportsABrokenFile(t *testing.T) {
	env := &Env{StateDir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(env.StateDir, "state.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := env.ReadStateJSON("state.json", &settings{}); err == nil {
		t.Error("ReadStateJSON() error = nil, want a decode error")
	}

	// An empty file reads as no state, the way an absent one does.
	if err := os.WriteFile(filepath.Join(env.StateDir, "empty.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := env.ReadStateJSON("empty.json", &settings{}); err != nil {
		t.Errorf("ReadStateJSON() error = %v, want nil for an empty file", err)
	}
}

func TestAppendStateJSONLKeepsConcurrentLinesWhole(t *testing.T) {
	env := &Env{StateDir: filepath.Join(t.TempDir(), "state")}

	const writers = 8
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := env.AppendStateJSONL("log.jsonl", settings{Theme: "dark", Limit: i}); err != nil {
				t.Errorf("AppendStateJSONL() error = %v", err)
			}
		}()
	}
	wg.Wait()

	data, err := env.ReadState("log.jsonl")
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != writers {
		t.Fatalf("lines = %d, want %d", len(lines), writers)
	}
	seen := make(map[int]bool, writers)
	for _, line := range lines {
		var entry settings
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %q does not decode: %v", line, err)
		}
		seen[entry.Limit] = true
	}
	if len(seen) != writers {
		t.Errorf("distinct records = %d, want %d", len(seen), writers)
	}
}

func TestWriteStateRawBytes(t *testing.T) {
	env := &Env{StateDir: filepath.Join(t.TempDir(), "state")}

	if err := env.WriteState("log.jsonl", []byte("{}\n")); err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}
	if err := env.AppendStateJSONL("log.jsonl", settings{Theme: "dark"}); err != nil {
		t.Fatalf("AppendStateJSONL() error = %v", err)
	}

	// Writing nothing empties the file, which is how a plugin starts a fresh
	// log for a new session.
	if err := env.WriteState("log.jsonl", nil); err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}
	data, err := env.ReadState("log.jsonl")
	if err != nil || len(data) != 0 {
		t.Errorf("ReadState() = %q, %v, want it emptied", data, err)
	}
}

func TestReadConfigJSON(t *testing.T) {
	dir := t.TempDir()
	env := &Env{ConfigDir: dir}

	// The defaults survive a plugin the user has never configured.
	cfg := settings{Theme: "dark", Limit: 5}
	if err := env.ReadConfigJSON("config.json", &cfg); err != nil {
		t.Fatalf("ReadConfigJSON() with no file = %v", err)
	}
	if cfg != (settings{Theme: "dark", Limit: 5}) {
		t.Errorf("config = %+v, want the defaults untouched", cfg)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"limit":9}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := env.ReadConfigJSON("config.json", &cfg); err != nil {
		t.Fatalf("ReadConfigJSON() = %v", err)
	}
	if cfg != (settings{Theme: "dark", Limit: 9}) {
		t.Errorf("config = %+v, want the file to override only what it sets", cfg)
	}
}

func TestConfigHelpersReportAMissingDirectoryAndABrokenFile(t *testing.T) {
	if _, err := (&Env{}).ReadConfig("config.json"); !errors.Is(err, ErrNoConfigDir) {
		t.Errorf("ReadConfig() = %v, want ErrNoConfigDir", err)
	}
	if err := (&Env{}).ReadConfigJSON("config.json", &settings{}); !errors.Is(err, ErrNoConfigDir) {
		t.Errorf("ReadConfigJSON() = %v, want ErrNoConfigDir", err)
	}

	env := &Env{ConfigDir: t.TempDir()}
	if _, err := env.ReadConfig(filepath.Join("..", "escape")); err == nil {
		t.Error("ReadConfig() accepted a name outside the directory")
	}
	if err := os.WriteFile(filepath.Join(env.ConfigDir, "config.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := env.ReadConfigJSON("config.json", &settings{})
	if err == nil || !strings.Contains(err.Error(), "decode config config.json") {
		t.Errorf("ReadConfigJSON() = %v, want it to name the file it could not decode", err)
	}
}
