package config

import (
	"encoding/json"
	"sync"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"gorm.io/gorm"
)

// SettingsStore is a read-through cache over the `settings` DB table.
type SettingsStore struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]string // key -> JSON value (raw)
}

// NewSettingsStore loads all settings rows into the cache.
func NewSettingsStore(db *gorm.DB) *SettingsStore {
	s := &SettingsStore{db: db, cache: make(map[string]string)}
	s.ReloadAll()
	return s
}

// Get unmarshals the cached JSON value for `key` into `dst`.
// A missing key leaves `dst` at its zero value and returns nil.
func (s *SettingsStore) Get(key string, dst interface{}) error {
	s.mu.RLock()
	raw, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

// Set marshals `value` to JSON, writes it to the DB, and updates the cache.
func (s *SettingsStore) Set(key string, value interface{}) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	raw := string(b)
	if err := database.SetSetting(s.db, key, raw); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[key] = raw
	s.mu.Unlock()
	return nil
}

// ReloadAll re-reads every settings row from the DB into the cache.
func (s *SettingsStore) ReloadAll() {
	rows, err := database.GetAllSettings(s.db)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]string, len(rows))
	for _, r := range rows {
		s.cache[r.Key] = r.Value
	}
}

// HasKey reports whether a settings row exists for `key` (used for
// bootstrapping defaults: e.g. a missing `auth.local` row = enabled).
func (s *SettingsStore) HasKey(key string) bool {
	s.mu.RLock()
	_, ok := s.cache[key]
	s.mu.RUnlock()
	return ok
}

// ListAll returns a copy of every cached setting (key -> JSON value).
func (s *SettingsStore) ListAll() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.cache))
	for k, v := range s.cache {
		out[k] = v
	}
	return out, nil
}

// DB exposes the underlying *gorm.DB (used by callers that need direct DB access).
func (s *SettingsStore) DB() *gorm.DB { return s.db }

// CORSConfig is the JSON shape of the `cors` settings row.
type CORSConfig struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// LoggerConfig is the JSON shape of the `logger` settings row.
type LoggerConfig struct {
	Level string `json:"level"`
	File  string `json:"file"`
}

// LocalAuthConfig is the JSON shape of the `auth.local` settings row.
type LocalAuthConfig struct {
	Enabled bool `json:"enabled"`
}

// GitLabConfig is the JSON shape of the `auth.gitlab` settings row.
type GitLabConfig struct {
	App                 string `json:"app"`
	URL                 string `json:"url"`
	ClientID            string `json:"client_id"`
	ClientSecret        string `json:"client_secret"`
	RedirectURI         string `json:"redirect_uri"`
	FrontendCallbackURL string `json:"frontend_callback_url"`
}
