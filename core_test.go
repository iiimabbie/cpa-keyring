package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func compiled(t *testing.T, r rule) rule {
	t.Helper()
	if err := compileRule(&r); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestHeadersScope(t *testing.T) {
	current.Lock()
	old := current.config
	current.config = config{Keys: []rule{{Scope: scopeOf("google")}, {Scope: scopeOf("anthropic")}}}
	current.Unlock()
	defer func() { current.Lock(); current.config = old; current.Unlock() }()
	h := http.Header{"Authorization": {"Bearer unknown"}, "X-Goog-Api-Key": {"google"}, "X-Api-Key": {"anthropic"}}
	if got := headersScope(h); got != scopeOf("google") {
		t.Fatalf("scope = %q", got)
	}
	h.Set("Authorization", "Bearer anthropic")
	if got := headersScope(h); got != scopeOf("anthropic") {
		t.Fatalf("precedence = %q", got)
	}
	h.Set("Authorization", "unknown")
	h.Del("X-Goog-Api-Key")
	h.Del("X-Api-Key")
	if got := headersScope(h); got != "" {
		t.Fatalf("unknown = %q", got)
	}
}
func TestGlobAndPolicy(t *testing.T) {
	r := compiled(t, rule{Allow: []string{"vendor/*"}, Deny: []string{"vendor/secre?"}})
	for _, tc := range []struct {
		s    string
		want bool
	}{{"vendor/a/b", true}, {"vendor/secret", false}, {"other", false}} {
		if got := allowed(r, tc.s); got != tc.want {
			t.Errorf("%s = %v", tc.s, got)
		}
	}
}
func TestModelLists(t *testing.T) {
	r := compiled(t, rule{Allow: []string{"keep"}})
	for _, body := range []string{`{"object":"list","data":[{"id":"keep"},{"id":"drop"}]}`, `{"data":[{"id":"keep"},{"id":"drop"}],"has_more":false,"first_id":"keep","last_id":"drop"}`, `{"models":[{"name":"models/keep"},{"name":"models/drop"}]}`, `{"models":[{"slug":"keep"},{"slug":"drop"}]}`} {
		out, ok, err := filterList([]byte(body), r)
		if err != nil || !ok {
			t.Fatalf("filter %s: %v", body, err)
		}
		var o map[string]json.RawMessage
		if err = json.Unmarshal(out, &o); err != nil {
			t.Fatal(err)
		}
		var a []json.RawMessage
		for _, k := range []string{"data", "models"} {
			if o[k] != nil {
				if err = json.Unmarshal(o[k], &a); err != nil {
					t.Fatal(err)
				}
			}
		}
		if len(a) != 1 {
			t.Fatalf("%s", out)
		}
	}
}
func TestPick(t *testing.T) {
	r := compiled(t, rule{Auths: []string{"a*"}})
	cs := []candidate{{ID: "a-low", Status: "active", Priority: 1}, {ID: "b", Status: "active", Priority: 20}, {ID: "a-high", Status: "active", Priority: 2}}
	if got := pick(r, cs)["AuthID"]; got != "a-high" {
		t.Fatal(got)
	}
	cs[2].Status = "disabled"
	cs[0].Status = "error"
	if got := pick(r, cs)["Reject"]; got != true {
		t.Fatal(got)
	}
	if got := pick(rule{}, cs)["Handled"]; got != false {
		t.Fatal(got)
	}
}

func TestPickFileName(t *testing.T) {
	got := pick(compiled(t, rule{Auths: []string{"team-?.json"}}), []candidate{{ID: "some/other-id", Status: "active", Attributes: map[string]string{"path": "/auth/team-a.json"}}})
	if got["AuthID"] != "some/other-id" {
		t.Fatal(got)
	}
}

func TestUnrelatedResponseUnchanged(t *testing.T) {
	if _, ok, err := filterList([]byte(`{"models":[{"name":"ordinary-object"}]}`), rule{Allow: []string{"allowed"}}); err != nil || ok {
		t.Fatalf("recognized unrelated response: %v %v", ok, err)
	}
}

func TestResponseMetadataScope(t *testing.T) {
	current.Lock()
	old := current.config
	current.config = config{Keys: []rule{compiled(t, rule{Scope: "scope", Allow: []string{"ok"}}), compiled(t, rule{Scope: scopeOf("other"), Allow: []string{"no"}})}}
	current.Unlock()
	defer func() { current.Lock(); current.config = old; current.Unlock() }()
	input, _ := json.Marshal(map[string]any{"Metadata": map[string]any{"caller_scope": "scope"}, "RequestHeaders": http.Header{"Authorization": []string{"Bearer other"}}, "Body": []byte(`{"object":"list","data":[{"id":"ok"},{"id":"no"}]}`)})
	result, err := dispatch("response.intercept_after", input)
	if err != nil {
		t.Fatal(err)
	}
	body := result.(map[string]any)["Body"].([]byte)
	var parsed struct {
		Data []any `json:"data"`
	}
	if err = json.Unmarshal(body, &parsed); err != nil || len(parsed.Data) != 1 {
		t.Fatalf("%s %v", body, err)
	}
}
func TestInvalidConfigRejected(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"schema_version": 6, "config_yaml": []byte("keys:\n  - scope: bad\n")})
	if err := configure(input); err == nil {
		t.Fatal("invalid scope accepted")
	}
}

func TestConfigureCompilesAndReplacesPatterns(t *testing.T) {
	scope := scopeOf("unit-key")
	apply := func(pattern string) {
		payload, _ := json.Marshal(map[string]any{"schema_version": 6, "config_yaml": []byte("keys:\n  - scope: " + scope + "\n    allow_models: [\"" + pattern + "\"]\n")})
		if err := configure(payload); err != nil {
			t.Fatal(err)
		}
	}
	apply("first/*")
	r, ok := policy(scope)
	if !ok || !allowed(r, "first/a") || allowed(r, "second/a") {
		t.Fatal("first policy not compiled")
	}
	apply("second/?")
	r, _ = policy(scope)
	if allowed(r, "first/a") || !allowed(r, "second/a") {
		t.Fatal("replacement policy not compiled")
	}
}
