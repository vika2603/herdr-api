package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// filePerm is the mode of the files the state helpers create. Herdr gives a
// plugin a directory under the user's own state home, so the files are
// readable by the user who can already read the directory.
const filePerm fs.FileMode = 0o644

// dirPerm is the mode of the state directory when a helper has to create it.
const dirPerm fs.FileMode = 0o755

// ErrNoStateDir is returned by the state helpers when Herdr set no
// HERDR_PLUGIN_STATE_DIR, which happens when a plugin command is run by hand
// outside Herdr.
var ErrNoStateDir = errors.New("plugin: HERDR_PLUGIN_STATE_DIR is not set")

// ErrNoConfigDir is returned by the configuration helpers when Herdr set no
// HERDR_PLUGIN_CONFIG_DIR.
var ErrNoConfigDir = errors.New("plugin: HERDR_PLUGIN_CONFIG_DIR is not set")

// StatePath joins name to the directory Herdr gives the plugin for its own
// state, HERDR_PLUGIN_STATE_DIR, and returns the state directory itself when
// called with no arguments.
//
// It returns the empty string when Herdr set no state directory and when name
// would leave it: an absolute element, an empty one, or enough ".." to climb
// out. The read and write helpers report those two cases as errors; a caller
// that builds a path itself has to check for the empty string.
func (e *Env) StatePath(name ...string) string {
	path, err := joinInside(e.StateDir, name)
	if err != nil {
		return ""
	}
	return path
}

// ConfigPath is StatePath for HERDR_PLUGIN_CONFIG_DIR, the directory holding
// the plugin's user-editable configuration.
func (e *Env) ConfigPath(name ...string) string {
	path, err := joinInside(e.ConfigDir, name)
	if err != nil {
		return ""
	}
	return path
}

// ReadState reads a file from the state directory. A file that does not exist
// yet is not an error: the result is nil, as it is for an empty file, because
// a plugin's first run finds no state either way.
func (e *Env) ReadState(name string) ([]byte, error) {
	path, err := e.statePath(name)
	if err != nil {
		return nil, err
	}
	return readFile(path, "state", name)
}

// readFile reports an absent file as no content, which both read helpers
// treat as "nothing written yet" rather than as a failure.
func readFile(path, kind, name string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s %s: %w", kind, name, err)
	}
	return data, nil
}

// ReadStateJSON decodes a JSON file from the state directory into into.
//
// An absent file is not an error and leaves into untouched, so a plugin can
// decode into a value already holding its defaults. An empty file is treated
// the same way, which is what a WriteStateJSON interrupted before its rename
// used to leave behind and what an author is most likely to create by hand.
func (e *Env) ReadStateJSON(name string, into any) error {
	data, err := e.ReadState(name)
	if err != nil || len(data) == 0 {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("plugin: decode state %s: %w", name, err)
	}
	return nil
}

// ReadConfig reads a file from the configuration directory, with the same
// contract as ReadState: an absent file is not an error and reads as nil,
// because a plugin the user has never configured is the normal case.
//
// There is no write counterpart. The configuration directory belongs to the
// user, and a plugin that rewrote it would discard their comments and their
// formatting; plugin-owned data belongs in the state directory.
func (e *Env) ReadConfig(name string) ([]byte, error) {
	path, err := e.configPath(name)
	if err != nil {
		return nil, err
	}
	return readFile(path, "config", name)
}

// ReadConfigJSON decodes a JSON file from the configuration directory into
// into, with the same contract as ReadStateJSON: an absent or empty file
// leaves into untouched, so a plugin decodes into a value already holding its
// defaults.
func (e *Env) ReadConfigJSON(name string, into any) error {
	data, err := e.ReadConfig(name)
	if err != nil || len(data) == 0 {
		return err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("plugin: decode config %s: %w", name, err)
	}
	return nil
}

// WriteState writes data to a file in the state directory, creating the
// directory when it does not exist.
//
// The write is atomic: the contents go to a temporary file in the same
// directory and are renamed over the target, so a crash or a full disk leaves
// either the previous file or the new one, never a truncated mix of the two.
// Two processes writing the same name concurrently both succeed and the last
// rename wins, so this suits a file one process owns rather than a log
// several append to; see AppendStateJSONL for that.
func (e *Env) WriteState(name string, data []byte) error {
	path, err := e.statePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("plugin: create state directory: %w", err)
	}
	return writeAtomic(path, data)
}

// WriteStateJSON writes value as JSON to a file in the state directory, with
// the guarantees WriteState documents.
func (e *Env) WriteStateJSON(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("plugin: encode state %s: %w", name, err)
	}
	return e.WriteState(name, data)
}

// AppendStateJSONL appends value to a file in the state directory as one JSON
// line, creating the file and the directory when they do not exist.
//
// Herdr runs one process per event hook and several may overlap, so a log is
// written by appending rather than by rewriting the whole file, which would
// lose the records a concurrent process wrote in between. The line is encoded
// first and appended in a single write, which keeps concurrent lines from
// interleaving.
func (e *Env) AppendStateJSONL(name string, value any) error {
	line, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("plugin: encode state %s: %w", name, err)
	}
	path, err := e.statePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("plugin: create state directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, filePerm)
	if err != nil {
		return fmt.Errorf("plugin: append state %s: %w", name, err)
	}
	_, writeErr := file.Write(append(line, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("plugin: append state %s: %w", name, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("plugin: append state %s: %w", name, closeErr)
	}
	return nil
}

// statePath resolves one name inside the state directory, distinguishing the
// two reasons StatePath returns the empty string.
func (e *Env) statePath(name string) (string, error) {
	return filePath(e.StateDir, "state", name, ErrNoStateDir)
}

func (e *Env) configPath(name string) (string, error) {
	return filePath(e.ConfigDir, "config", name, ErrNoConfigDir)
}

func filePath(dir, kind, name string, absent error) (string, error) {
	if dir == "" {
		return "", absent
	}
	path, err := joinInside(dir, []string{name})
	if err != nil {
		return "", fmt.Errorf("plugin: %s file %q: %w", kind, name, err)
	}
	return path, nil
}

// errEscapes reports a name that would resolve outside the directory it is
// joined to.
var errEscapes = errors.New("name must stay inside the plugin directory")

// joinInside joins name to dir and checks that the result stays under dir.
// The check is textual, on the cleaned path, so a symlink inside dir can
// still point elsewhere; it catches the name a plugin builds from data, not
// an adversary with write access to the plugin's own directory.
func joinInside(dir string, name []string) (string, error) {
	if dir == "" {
		return "", errors.New("directory is not set")
	}
	if len(name) == 0 {
		return dir, nil
	}
	for _, element := range name {
		if element == "" || filepath.IsAbs(element) || filepath.VolumeName(element) != "" {
			return "", errEscapes
		}
	}
	joined := filepath.Join(append([]string{dir}, name...)...)
	relative, err := filepath.Rel(filepath.Clean(dir), joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errEscapes
	}
	return joined, nil
}

// writeAtomic writes data to path through a temporary file in the same
// directory. The temporary file is removed when anything before the rename
// fails, so a failed write leaves nothing behind.
func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("plugin: create temporary file for %s: %w", path, err)
	}
	temp := file.Name()
	if err := fillTemp(file, data); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("plugin: write %s: %w", path, err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("plugin: replace %s: %w", path, err)
	}
	return nil
}

// fillTemp writes data to file and closes it. The contents are flushed before
// the caller renames, because the rename only protects the previous file if
// the new one reached the file system first.
func fillTemp(file *os.File, data []byte) (err error) {
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(filePerm); err != nil {
		return err
	}
	return file.Sync()
}
