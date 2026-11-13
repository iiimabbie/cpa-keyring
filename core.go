package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type rule struct {
	Scope                                     string   `json:"scope" yaml:"scope"`
	Name                                      string   `json:"name,omitempty" yaml:"name"`
	Note                                      string   `json:"note,omitempty" yaml:"note"`
	Auths                                     []string `json:"auths,omitempty" yaml:"auths"`
	Allow                                     []string `json:"allow_models,omitempty" yaml:"allow_models"`
	Deny                                      []string `json:"deny_models,omitempty" yaml:"deny_models"`
	authPatterns, allowPatterns, denyPatterns []*regexp.Regexp
}
type config struct {
	Enabled  bool   `yaml:"enabled"`
	Priority int    `yaml:"priority"`
	Store    any    `yaml:"store"`
	Keys     []rule `yaml:"keys"`
}

var current struct {
	sync.RWMutex
	config config
}

func scopeOf(key string) string {
	h := sha256.Sum256([]byte("cli-proxy-api:caller-scope:v1\x00" + key))
	return hex.EncodeToString(h[:])
}
func configure(raw []byte) error {
	var req struct {
		ConfigYAML    []byte `json:"config_yaml"`
		SchemaVersion int    `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	if req.SchemaVersion < 2 {
		return fmt.Errorf("keyring requires plugin schema version 2 or newer")
	}
	var cfg config
	d := yaml.NewDecoder(bytes.NewReader(req.ConfigYAML))
	d.KnownFields(true)
	if err := d.Decode(&cfg); err != nil {
		return fmt.Errorf("keyring configuration: %w", err)
	}
	seen := map[string]bool{}
	for i := range cfg.Keys {
		r := &cfg.Keys[i]
		b, e := hex.DecodeString(r.Scope)
		if e != nil || len(b) != 32 || r.Scope != strings.ToLower(r.Scope) || seen[r.Scope] {
			return fmt.Errorf("invalid or duplicate caller scope %q", r.Scope)
		}
		seen[r.Scope] = true
		if err := compileRule(r); err != nil {
			return err
		}
	}
	current.Lock()
	current.config = cfg
	current.Unlock()
	return nil
}
func policy(scope string) (rule, bool) {
	current.RLock()
	defer current.RUnlock()
	for _, r := range current.config.Keys {
		if r.Scope == scope {
			return r, true
		}
	}
	return rule{}, false
}
func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if p == "" {
			return nil, fmt.Errorf("empty glob")
		}
		q := regexp.QuoteMeta(p)
		q = strings.ReplaceAll(q, `\*`, ".*")
		q = strings.ReplaceAll(q, `\?`, ".")
		re, err := regexp.Compile("^(?:" + q + ")$")
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}
func compileRule(r *rule) error {
	var err error
	if r.authPatterns, err = compilePatterns(r.Auths); err != nil {
		return err
	}
	if r.allowPatterns, err = compilePatterns(r.Allow); err != nil {
		return err
	}
	r.denyPatterns, err = compilePatterns(r.Deny)
	return err
}
func match(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}
func allowed(r rule, model string) bool {
	return !match(r.denyPatterns, model) && (len(r.allowPatterns) == 0 || match(r.allowPatterns, model))
}
func headersScope(h http.Header) string {
	auth := h.Get("Authorization")
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		auth = strings.TrimSpace(parts[1])
	}
	for _, v := range []string{auth, h.Get("X-Goog-Api-Key"), h.Get("X-Api-Key")} {
		if v != "" {
			scope := scopeOf(v)
			// Only caller scopes with configured rules are eligible for header-based filtering.
			if _, ok := policy(scope); ok {
				return scope
			}
		}
	}
	return ""
}

type candidate struct {
	ID, Status string
	Priority   int
	Attributes map[string]string
}

func pick(r rule, cs []candidate) map[string]any {
	if len(r.Auths) == 0 {
		return map[string]any{"Handled": false}
	}
	var best *candidate
	for i := range cs {
		c := &cs[i]
		if c.Status != "active" && c.Status != "" {
			continue
		}
		if !match(r.authPatterns, c.ID) && !match(r.authPatterns, filepath.Base(c.Attributes["path"])) {
			continue
		}
		if best == nil || c.Priority > best.Priority {
			best = c
		}
	}
	if best == nil {
		return map[string]any{"Handled": true, "Reject": true, "RejectReason": "keyring: no allowed upstream credential is available", "RejectCode": "auth_unavailable"}
	}
	return map[string]any{"Handled": true, "AuthID": best.ID}
}
func uncloak(id string) string {
	const prefix = "claude-fable-5-dd-"
	if !strings.HasPrefix(id, prefix) {
		return id
	}
	r := []rune(strings.TrimPrefix(id, prefix))
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
func filterList(body []byte, r rule) ([]byte, bool, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil {
		return nil, false, nil
	}
	field, ident := "", ""
	var marker string
	_ = json.Unmarshal(obj["object"], &marker)
	if marker == "list" {
		field, ident = "data", "id"
	} else if _, ok := obj["has_more"]; ok && obj["first_id"] != nil && obj["last_id"] != nil {
		field, ident = "data", "id"
	} else if obj["models"] != nil {
		field, ident = "models", "name"
	}
	if field == "" {
		return nil, false, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(obj[field], &entries); err != nil {
		return nil, false, nil
	}
	filtered := make([]json.RawMessage, 0, len(entries))
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		var e map[string]json.RawMessage
		if json.Unmarshal(entry, &e) != nil {
			return nil, false, nil
		}
		var id string
		_ = json.Unmarshal(e[ident], &id)
		if field == "models" {
			if id != "" && !strings.HasPrefix(id, "models/") {
				return nil, false, nil
			}
			if id == "" {
				_ = json.Unmarshal(e["slug"], &id)
			}
		}
		if id == "" {
			return nil, false, nil
		}
		name := strings.TrimPrefix(id, "models/")
		if field == "data" && obj["has_more"] != nil {
			name = uncloak(name)
		}
		if allowed(r, name) {
			filtered = append(filtered, entry)
			ids = append(ids, id)
		}
	}
	obj[field], _ = json.Marshal(filtered)
	if obj["has_more"] != nil && field == "data" {
		first, last := "", ""
		if len(ids) > 0 {
			first = ids[0]
			last = ids[len(ids)-1]
		}
		obj["first_id"], _ = json.Marshal(first)
		obj["last_id"], _ = json.Marshal(last)
	}
	out, err := json.Marshal(obj)
	return out, true, err
}
