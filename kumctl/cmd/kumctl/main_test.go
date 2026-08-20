package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRequestCoversAPICommandSurface(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		verb      string
		resource  string
		id        string
		opts      requestOptions
		method    string
		path      string
		mutation  bool
		body      string
		wantError string
	}{
		{name: "login credentials", verb: "login", resource: "login", opts: requestOptions{username: "alice", password: "secret"}, method: http.MethodPost, path: "/auth/login", body: `{"username":"alice","password":"secret"}`},
		{name: "list projects by module", verb: "list", resource: "projects", opts: requestOptions{moduleID: "mod/a", page: 2, size: 10}, method: http.MethodGet, path: "/projects/module/mod%2Fa?page=2&size=10"},
		{name: "get module children", verb: "get", resource: "modules", id: "m1", opts: requestOptions{children: true}, method: http.MethodGet, path: "/modules/m1/children"},
		{name: "get role permissions", verb: "get", resource: "roles", id: "2", opts: requestOptions{permissions: true}, method: http.MethodGet, path: "/roles/2/permissions"},
		{name: "create application", verb: "create", resource: "applications", opts: requestOptions{file: "testdata/application.json"}, method: http.MethodPost, path: "/applications", mutation: true},
		{name: "update module", verb: "update", resource: "modules", id: "m/1", opts: requestOptions{file: "testdata/application.json"}, method: http.MethodPut, path: "/modules/m%2F1", mutation: true},
		{name: "delete project", verb: "delete", resource: "projects", id: "p1", method: http.MethodDelete, path: "/projects/p1", mutation: true},
		{name: "adopt cluster", verb: "adopt", resource: "clusters", id: "c1", method: http.MethodPost, path: "/clusters/c1/adopt", mutation: true},
		{name: "adopt workspace", verb: "adopt", resource: "workspaces", id: "w1", opts: requestOptions{file: "testdata/application.json"}, method: http.MethodPost, path: "/workspaces/w1/adopt-existing", mutation: true},
		{name: "register rejected by command layer", verb: "register", resource: "auth", wantError: "unsupported verb \"register\""},
		{name: "role create rejected", verb: "create", resource: "roles", wantError: "create is not supported for roles"},
		{name: "operation list rejected", verb: "list", resource: "operations", wantError: "list is not supported for operations"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			if opts.file != "" {
				dir := t.TempDir()
				path := filepath.Join(dir, "request.json")
				if err := os.WriteFile(path, []byte(`{"name":"demo"}`), 0o600); err != nil {
					t.Fatal(err)
				}
				opts.file = path
			}
			method, path, body, mutation, err := buildRequest(tt.verb, tt.resource, tt.id, opts)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if method != tt.method || path != tt.path || mutation != tt.mutation {
				t.Fatalf("request = %s %s mutation=%v, want %s %s mutation=%v", method, path, mutation, tt.method, tt.path, tt.mutation)
			}
			if tt.body != "" && string(body) != tt.body {
				t.Fatalf("body = %s, want %s", body, tt.body)
			}
		})
	}
}

func TestBuildRequestSupportsEveryAPICRUDResource(t *testing.T) {
	resources := []string{"users", "modules", "projects", "workspaces", "applications"}
	for _, resource := range resources {
		for _, verb := range []string{"create", "update", "delete"} {
			t.Run(verb+"/"+resource, func(t *testing.T) {
				opts := requestOptions{}
				if verb != "delete" {
					path := filepath.Join(t.TempDir(), "request.json")
					if err := os.WriteFile(path, []byte(`{"name":"demo"}`), 0o600); err != nil {
						t.Fatal(err)
					}
					opts.file = path
				}
				id := "id1"
				method, path, _, mutation, err := buildRequest(verb, resource, id, opts)
				if err != nil {
					t.Fatal(err)
				}
				if !mutation || (verb == "create" && method != http.MethodPost) ||
					(verb == "update" && method != http.MethodPut) ||
					(verb == "delete" && method != http.MethodDelete) {
					t.Fatalf("request = %s %s mutation=%v", method, path, mutation)
				}
			})
		}
	}
}

func TestParsePositionalsAndAliases(t *testing.T) {
	verb, resource, id, err := parsePositionals([]string{"get", "module", "m1"})
	if err != nil || verb != "get" || resource != "module" || id != "m1" {
		t.Fatalf("parsed = %q %q %q, err=%v", verb, resource, id, err)
	}
	if got := canonicalResource(resource); got != "modules" {
		t.Fatalf("canonical resource = %q", got)
	}
	verb, resource, _, err = parsePositionals([]string{"auth", "login"})
	if err != nil || verb != "auth" || resource != "login" {
		t.Fatalf("auth login parsed = %q %q, err=%v", verb, resource, err)
	}
}

func TestNormalizeFlagsAllowsKubectlStylePlacement(t *testing.T) {
	got := normalizeFlags([]string{"create", "modules", "m1", "--file", "module.json", "--wait"})
	want := []string{"--file", "module.json", "--wait", "create", "modules", "m1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("normalized = %#v, want %#v", got, want)
	}
	got = normalizeFlags([]string{"create", "modules", "-f", "module.json"})
	if strings.Join(got, " ") != "-f module.json create modules" {
		t.Fatalf("short file normalized = %#v", got)
	}
}

func TestRunRejectsExcludedAuthOperations(t *testing.T) {
	for _, operation := range []string{"register", "me", "change-password"} {
		err := run([]string{"auth", operation}, io.Discard, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "kumctl does not expose auth/"+operation) {
			t.Errorf("%s error = %v", operation, err)
		}
	}
}

func TestRunSendsHTTPRequestAndPrintsResponse(t *testing.T) {
	var got struct {
		method string
		path   string
		body   map[string]any
		token  string
		key    string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.token, got.key = r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"message":"accepted","data":{"id":"op1","state":"pending"}}`)
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(file, []byte(`{"name":"demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := run([]string{"create", "modules", "--server", srv.URL, "--token", "tok", "-f", file}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPost || got.path != "/api/v1/modules" || got.token != "Bearer tok" || got.key == "" {
		t.Fatalf("request metadata = %+v", got)
	}
	if got.body["name"] != "demo" || !strings.Contains(out.String(), `"op1"`) {
		t.Fatalf("body/output = %+v / %s", got.body, out.String())
	}
}
