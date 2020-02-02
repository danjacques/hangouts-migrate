package attachment

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/danjacques/hangouts-migrate/util"
)

var Exists = errors.New("Already exists")
var NotFound = errors.New("Not Found")

type Mapper struct {
	BasePath  string
	Overwrite bool

	attachments sync.Map
}

func (m *Mapper) LoadFromJSON(r io.Reader) error {
	var of outputFormat
	dec := json.NewDecoder(r)
	if err := dec.Decode(&of); err != nil {
		return err
	}
	for k, v := range of.Entries {
		if _, err := os.Stat(v); err != nil {
			if os.IsNotExist(err) {
				log.Printf("Entry for %s does not exist; discarding: %s", k, v)
				continue
			} else {
				return fmt.Errorf("failed to stat key %s, path %s: %w", k, v, err)
			}
		}
		m.attachments.Store(k, v)
	}
	return nil
}

func (m *Mapper) SaveToJSON(w io.Writer) error {
	of := outputFormat{
		Entries: make(map[string]string),
	}
	m.attachments.Range(func(k, v interface{}) bool {
		of.Entries[k.(string)] = v.(string)
		return true
	})
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&of)
}

func (m *Mapper) Mapped(key string) bool {
	_, ok := m.attachments.Load(key)
	return ok
}

func (m *Mapper) OpenWriter(key string, mediaType string) (*Writer, error) {
	if m.BasePath == "" {
		return nil, errors.New("cannot write files, no base path set")
	}

	name := util.HashForKey(key)
	if ext := extensionForMediaType(mediaType); ext != "" {
		name = fmt.Sprintf("%s.%s", name, ext)
	}

	// Always store the path, even if we don't end up writing it.
	path := filepath.Join(m.BasePath, name)
	if _, ok := m.attachments.LoadOrStore(key, path); ok {
		// An entry already exists.
		return nil, Exists
	}

	if !m.Overwrite {
		if _, err := os.Stat(path); err == nil {
			// Destination already exists.
			return nil, Exists
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("checking for %s: %w", path, err)
		}
	}

	return makeWriter(path)
}

func (m *Mapper) GetPath(key string) string {
	if v, ok := m.attachments.Load(key); ok {
		return v.(string)
	}
	return ""
}

func (m *Mapper) ScanPathForKey(key string) (string, error) {
	if path := m.GetPath(key); path != "" {
		// Already in m.
		return path, nil
	}

	if m.BasePath == "" {
		return "", NotFound
	}

	// <hash>*
	pathGlob := filepath.Join(m.BasePath, util.HashForKey(key)+"*")
	matches, err := filepath.Glob(pathGlob)
	if err != nil {
		return "", fmt.Errorf("failed to glob %s: %w", pathGlob, err)
	}
	if len(matches) > 0 {
		v, _ := m.attachments.LoadOrStore(key, matches[0])
		return v.(string), nil
	}
	return "", NotFound
}

var autoPrefixes = []string{"image/", "video/"}

func extensionForMediaType(mt string) string {
	switch mt {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	default:
		for _, ap := range autoPrefixes {
			if strings.HasPrefix(mt, ap) {
				return strings.TrimPrefix(mt, ap)
			}
		}
		return ""
	}
}
