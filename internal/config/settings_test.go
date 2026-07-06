package config

import (
	"encoding/json"
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database"
)

func newTestStore(t *testing.T) *SettingsStore {
	t.Helper()
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return NewSettingsStore(db)
}

func TestSettingsStore_SetGet_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	type corsVal struct{ AllowedOrigins []string }
	want := corsVal{AllowedOrigins: []string{"https://oj.example.com", "*"}}
	if err := s.Set("cors", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var got corsVal
	if err := s.Get("cors", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.AllowedOrigins) != 2 || got.AllowedOrigins[0] != "https://oj.example.com" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSettingsStore_Get_Missing(t *testing.T) {
	s := newTestStore(t)
	var got struct{ X int }
	if err := s.Get("nope", &got); err != nil {
		t.Fatalf("Get missing should not error: %v", err)
	}
	if got.X != 0 {
		t.Errorf("missing key should yield zero value; got %+v", got)
	}
}

func TestSettingsStore_ReloadAll(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("k", "v1")
	// Mutate the DB behind the cache.
	if err := database.SetSetting(s.db, "k", `"v2"`); err != nil {
		t.Fatalf("direct SetSetting: %v", err)
	}
	// Cache still has v1.
	var cached string
	_ = s.Get("k", &cached)
	if cached != "v1" {
		t.Errorf("cache should have v1 before reload; got %s", cached)
	}
	s.ReloadAll()
	var after string
	_ = s.Get("k", &after)
	if after != "v2" {
		t.Errorf("after reload should have v2; got %s", after)
	}
}

// keep encoding/json referenced if not used directly above
var _ = json.Marshal
