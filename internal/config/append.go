package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Append adds inst to the end of the config file at path, creating the file
// and its folder if missing. The text already there, comments included, is
// kept as it is. The file is replaced in one step, so a failure leaves it as
// it was; a symlinked config stays a link to the updated file.
func Append(path string, inst Instance) error {
	// Write to the file a symlinked config points to, keeping the link.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	old, err := os.ReadFile(path)
	mode := fs.FileMode(0o600)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
	}

	entry, err := encodeInstance(inst)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.Write(old)
	if len(old) > 0 {
		if !bytes.HasSuffix(old, []byte("\n")) {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	buf.Write(entry)
	var check Config
	if _, err := toml.Decode(buf.String(), &check); err != nil {
		return fmt.Errorf("add instance %q to %s: %w", inst.Name, path, err)
	}
	return replaceFile(path, buf.Bytes(), mode)
}

// encodeInstance writes inst as one [[instance]] table, leaving out the keys
// it does not set.
func encodeInstance(inst Instance) ([]byte, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	err := enc.Encode(struct {
		Instance []Instance `toml:"instance"`
	}{[]Instance{inst}})
	return buf.Bytes(), err
}

// replaceFile writes data to a new file next to path and renames it over
// path.
func replaceFile(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
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
	return os.Rename(tmp.Name(), path)
}
