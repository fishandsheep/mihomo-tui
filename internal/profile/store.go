package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	envConfigPath = "MIHOMO_TUI_CONFIG"
)

type Profile struct {
	Name          string `json:"name"`
	ControllerURL string `json:"controller_url"`
	Secret        string `json:"secret"`
	TLSSkipVerify bool   `json:"tls_skip_verify"`
	Default       bool   `json:"default,omitempty"`
}

type fileState struct {
	Profiles []Profile `json:"profiles"`
}

type Store struct {
	path  string
	state fileState
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		path = os.Getenv(envConfigPath)
	}
	if path == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(configDir, "mihomo-tui", "profiles.json")
	}

	store := &Store{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) List() []Profile {
	out := append([]Profile(nil), s.state.Profiles...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Default != out[j].Default {
			return out[i].Default
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *Store) Get(name string) (Profile, bool) {
	for _, item := range s.state.Profiles {
		if item.Name == name {
			return item, true
		}
	}
	return Profile{}, false
}

func (s *Store) Default() (Profile, bool) {
	for _, item := range s.state.Profiles {
		if item.Default {
			return item, true
		}
	}
	if len(s.state.Profiles) == 1 {
		return s.state.Profiles[0], true
	}
	return Profile{}, false
}

func (s *Store) Upsert(p Profile) error {
	if p.Name == "" {
		return errors.New("profile name required")
	}
	if p.ControllerURL == "" {
		return errors.New("controller URL required")
	}

	found := false
	for i, item := range s.state.Profiles {
		if item.Name == p.Name {
			s.state.Profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		s.state.Profiles = append(s.state.Profiles, p)
	}
	if p.Default {
		for i := range s.state.Profiles {
			if s.state.Profiles[i].Name != p.Name {
				s.state.Profiles[i].Default = false
			}
		}
	}
	return s.save()
}

func (s *Store) Remove(name string) error {
	next := make([]Profile, 0, len(s.state.Profiles))
	removed := false
	removedDefault := false
	for _, item := range s.state.Profiles {
		if item.Name == name {
			removed = true
			removedDefault = item.Default
			continue
		}
		next = append(next, item)
	}
	if !removed {
		return fmt.Errorf("profile %q not found", name)
	}
	if removedDefault && len(next) > 0 {
		next[0].Default = true
	}
	s.state.Profiles = next
	return s.save()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.state = fileState{}
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.state = fileState{}
		return nil
	}
	return json.Unmarshal(data, &s.state)
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
