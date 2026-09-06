package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Deployment is the durable audit-facing release object shared by Git
// activation and sandboxed builds. It intentionally records independently
// completed stages; later orchestration can compose those stages without
// erasing their actor, commit, artifact, or rollback provenance.
type Deployment struct {
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

type DeploymentStore struct {
	mu     sync.RWMutex
	path   string
	values []Deployment
}

func OpenDeploymentStore(path string) (*DeploymentStore, error) {
	s := &DeploymentStore{path: path, values: []Deployment{}}
	d, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(d, &s.values); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *DeploymentStore) add(item Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = append([]Deployment{item}, s.values...)
	if len(s.values) > 1000 {
		s.values = s.values[:1000]
	}
	d, e := json.MarshalIndent(s.values, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomic(s.path, append(d, '\n'), 0600)
}
func (s *DeploymentStore) list(site string) []Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Deployment{}
	for _, v := range s.values {
		if site == "" || v.Site == site {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (a *App) recordDeployment(site, stage, state, detail string, result gitDeployResult, artifact string) {
	if a.Deployments == nil {
		return
	}
	id, _ := newJobID("deployment")
	_ = a.Deployments.add(Deployment{ID: id, Site: site, Repository: result.Repository, Ref: result.Ref, Commit: result.Commit, Stage: stage, State: state, Detail: detail, Artifact: artifact, Previous: result.Previous, CreatedAt: time.Now().UTC()})
}
func (a *App) deployments(w http.ResponseWriter, r *http.Request) {
	site := safeUser(strings.TrimSpace(r.URL.Query().Get("site")))
	if strings.TrimSpace(r.URL.Query().Get("site")) != "" && site == "" {
		http.Error(w, "invalid site", 422)
		return
	}
	if site != "" && !a.canAccessSite(r, site) {
		http.Error(w, "site is not assigned to this account", 403)
		return
	}
	items := a.Deployments.list(site)
	if !a.Auth.IsAdministrator(r) && site == "" {
		filtered := items[:0]
		for _, item := range items {
			if a.canAccessSite(r, item.Site) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	writeJSON(w, 200, map[string]any{"deployments": items})
}
