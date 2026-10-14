package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Settings(); got.Brightness != 50 || got.VideoBrightness != 100 {
		t.Errorf("got %+v", got)
	}
}

func TestOldFileGetsVideoBrightness(t *testing.T) {
	dir := t.TempDir()
	old := `{"settings":{"automationEnabled":true,"onTime":"07:00","offTime":"22:00","color":255,"brightness":40}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Settings(); got.Brightness != 40 || got.VideoBrightness != 100 {
		t.Errorf("got %+v", got)
	}
}

func TestSaveIsPrivate(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	if _, err := s.UpdateSettings(func(st *Settings) { st.Color = 1 }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "state.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, err %v", info.Mode().Perm(), err)
	}
}
