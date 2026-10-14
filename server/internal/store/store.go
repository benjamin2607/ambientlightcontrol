// Package store persists settings and the TV pairing in a JSON file inside the config directory.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/benjamin2607/ambientlightcontrol/server/internal/webos"
)

// SceneRef selects the Govee snapshot or DIY scene used for "video" mode, by name.
type SceneRef struct {
	Kind string `json:"kind"` // "snapshot" or "diyScene"
	Name string `json:"name"`
}

type Settings struct {
	AutomationEnabled bool      `json:"automationEnabled"`
	OnTime            string    `json:"onTime"`
	OffTime           string    `json:"offTime"`
	Color             int       `json:"color"`           // 0xRRGGBB, the fixed daytime colour
	Brightness        int       `json:"brightness"`      // 1–100, with the daytime colour
	VideoBrightness   int       `json:"videoBrightness"` // 1–100, in video mode
	VideoSource       *SceneRef `json:"videoSource,omitempty"`
	TVHost            string    `json:"tvHost"`
}

func DefaultSettings() Settings {
	return Settings{
		AutomationEnabled: true,
		OnTime:            "07:00",
		OffTime:           "22:00",
		Color:             0xFF9A3C,
		Brightness:        50,
		VideoBrightness:   100,
	}
}

type data struct {
	Settings Settings          `json:"settings"`
	TV       webos.Credentials `json:"tv"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data data
}

// Open loads the state file or starts with defaults if it does not exist yet.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "state.json"), data: data{Settings: DefaultSettings()}}
	raw, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	if s.data.Settings.VideoBrightness == 0 { // files from version 1.0.0
		s.data.Settings.VideoBrightness = DefaultSettings().VideoBrightness
	}
	return s, nil
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}

func (s *Store) TV() webos.Credentials {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.TV
}

func (s *Store) UpdateSettings(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data.Settings)
	return s.data.Settings, s.save()
}

func (s *Store) SetTV(c webos.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.TV = c
	return s.save()
}

// save writes atomically with owner-only permissions (the file contains the TV pairing key).
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
