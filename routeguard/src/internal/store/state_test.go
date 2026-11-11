package store

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ZeeeUs/RouteGuard/internal/config"
)

//go:embed testdata/legacy-state.json
var legacyFixture []byte

//go:embed testdata/legacy-obsolete-config.json
var obsoleteConfigFixture []byte

func TestVersionedStateAndJournal(t *testing.T) {
	files := Files{Path: filepath.Join(t.TempDir(), "guard", "state.json")}
	state := State{Version: Version, Generation: 4, Config: &config.Config{Settings: config.Settings{Enabled: true}}}
	if err := files.Prepare(state); err != nil {
		t.Fatal(err)
	}
	pending, err := files.Pending()
	if err != nil || !reflect.DeepEqual(pending, &state) {
		t.Fatalf("journal=%+v,%v", pending, err)
	}
	if err := files.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := files.Complete(); err != nil {
		t.Fatal(err)
	}
	got, exists, err := files.Load()
	if err != nil || !exists || !reflect.DeepEqual(got, state) {
		t.Fatalf("saved=%+v,%v", got, err)
	}
	if pending, err := files.Pending(); pending != nil || err != nil {
		t.Fatalf("completed journal=%v,%v", pending, err)
	}
}

func TestLegacyMigration(t *testing.T) {
	files := Files{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := os.WriteFile(files.Path, legacyFixture, 0600); err != nil {
		t.Fatal(err)
	}
	state, exists, err := files.Load()
	if err != nil || !exists || state.Version != Version || !state.Config.Settings.Enabled || state.Config.Services[0].Label != "Alice's # service" {
		t.Fatalf("migration=%+v,%v", state, err)
	}
}

func TestFutureVersionIsPreserved(t *testing.T) {
	files := Files{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := WriteJSON(files.Path, State{Version: Version + 1}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(files.Path)
	if _, _, err := files.Load(); err == nil {
		t.Fatal("accepted future version")
	}
	after, _ := os.ReadFile(files.Path)
	if string(before) != string(after) {
		t.Fatal("unrecognized state was modified")
	}
}

func TestLegacyRulesSurviveObsoleteConfiguration(t *testing.T) {
	files := Files{Path: filepath.Join(t.TempDir(), "state.json")}
	if err := os.WriteFile(files.Path, obsoleteConfigFixture, 0600); err != nil {
		t.Fatal(err)
	}

	state, exists, err := files.Load()
	if err != nil || !exists || state.Active == nil {
		t.Fatalf("saved rules were lost: %+v, %v", state, err)
	}

	if state.Config != nil || !state.Paused || state.MigrationWarning == "" {
		t.Fatalf("obsolete configuration was accepted silently: %+v", state)
	}

	if got := state.Active.Plan.Targets; len(got) != 1 || got[0].String() != "192.0.2.10/32" {
		t.Fatalf("saved targets changed: %v", got)
	}
}

func TestAtomicWriteRetainsExistingFileOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	err := AtomicWrite(path, func(file *os.File) error {
		_, _ = file.WriteString("partial new content")
		return errors.New("write failure")
	})
	data, _ := os.ReadFile(path)
	if err == nil || string(data) != "old" {
		t.Fatalf("partial write became visible: %q,%v", data, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary files leaked")
	}
}
