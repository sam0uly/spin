package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// defaultConfigDir returns the default config root (~/.config/spin).
func defaultConfigDir() string {
	cache, _ := os.UserConfigDir()
	if cache == "" {
		cache = "."
	}
	return filepath.Join(cache, "spin")
}

// atomicWriteJSON marshals data as indented JSON and writes it to
// path atomically via a temp file and rename, so a crash mid-write
// cannot leave a corrupt file behind.
func atomicWriteJSON(path string, data any, tmpPrefix string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, tmpPrefix)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
