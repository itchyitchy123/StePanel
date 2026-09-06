// Package deployment owns durable deployment-history state. It deliberately
// has no HTTP or platform-helper dependencies so it can be tested and reused
// by future release orchestration code.
package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	statefile "github.com/itchyitchy123/StePanel/internal/state"
)

type Record struct {
	ID         string    `json:"id"`
	Site       string    `json:"site"`
	Repository string    `json:"repository,omitempty"`
	Ref        string    `json:"ref,omitempty"`
	Commit     string    `json:"commit,omitempty"`
	Stage      string    `json:"stage"`
	State      string    `json:"state"`
	Artifact   string    `json:"artifact,omitempty"`
	Previous   string    `json:"previous_release,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Store struct {
	mu     sync.RWMutex
	path   string
	values []Record
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, values: []Record{}}
	d, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(d, &s.values); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Add(record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]Record(nil), s.values...)
	s.values = append([]Record{record}, s.values...)
	if len(s.values) > 1000 {
		s.values = s.values[:1000]
	}
	data, err := json.MarshalIndent(s.values, "", "  ")
	if err != nil {
		return err
	}
	if err := statefile.WriteAtomic(s.path, append(data, '\n'), 0600); err != nil {
		s.values = previous
		return err
	}
	return nil
}

func (s *Store) List(site string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.values))
	for _, record := range s.values {
		if site == "" || record.Site == site {
			out = append(out, record)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
