package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	api "github.com/fize/kumquat/kumctl/internal/client"
)

type settings struct {
	CurrentContext string              `json:"currentContext"`
	Contexts       map[string]endpoint `json:"contexts"`
}

type endpoint struct {
	Server string `json:"server"`
	Token  string `json:"token,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func configPath() string {
	if p := os.Getenv("KUMQUAT_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kumquat", "config.json")
}

func loadSettings() settings {
	var s settings
	s.Contexts = map[string]endpoint{}
	b, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Contexts == nil {
		s.Contexts = map[string]endpoint{}
	}
	return s
}

func mutationKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "kumctl-" + hex.EncodeToString(b)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out, errOut io.Writer) error {
	args = normalizeFlags(args)
	fs := flag.NewFlagSet("kumctl", flag.ContinueOnError)
	fs.SetOutput(errOut)
	server := fs.String("server", os.Getenv("KUMQUAT_API_URL"), "Kumquat API base URL")
	token := fs.String("token", os.Getenv("KUMQUAT_TOKEN"), "bearer token")
	contextName := fs.String("context", "", "configured context")
	output := fs.String("output", "json", "output format (json)")
	wait := fs.Bool("wait", false, "wait for asynchronous operation completion")
	timeout := fs.Duration("timeout", 2*time.Minute, "wait timeout")
	key := fs.String("idempotency-key", "", "mutation idempotency key")
	file := fs.String("file", "", "JSON request file")
	fs.StringVar(file, "f", "", "JSON request file (shorthand for --file)")
	fs.StringVar(output, "o", "json", "output format (shorthand for --output)")
	username := fs.String("username", "", "username for login")
	password := fs.String("password", "", "password for login")
	moduleID := fs.String("module-id", "", "module public ID for project listing")
	children := fs.Bool("children", false, "list module children")
	permissions := fs.Bool("permissions", false, "list role permissions")
	page := fs.Int("page", 0, "page number")
	size := fs.Int("size", 0, "page size")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output != "json" {
		return fmt.Errorf("only --output json is supported")
	}

	pos := fs.Args()
	if len(pos) == 0 {
		return usageError()
	}

	s := loadSettings()
	selected := *contextName
	if selected == "" {
		selected = s.CurrentContext
	}
	if ep, ok := s.Contexts[selected]; ok {
		if *server == "" {
			*server = ep.Server
		}
		if *token == "" {
			*token = ep.Token
		}
	}
	if *server == "" {
		*server = "http://127.0.0.1:8080"
	}

	verb, resource, id, err := parsePositionals(pos)
	if err != nil {
		return err
	}
	if verb == "register" || verb == "me" || verb == "change-password" {
		return fmt.Errorf("kumctl does not expose auth/%s", verb)
	}
	if verb == "auth" && resource != "login" {
		return fmt.Errorf("kumctl does not expose auth/%s", resource)
	}
	if verb != "auth" && verb != "login" {
		resource = canonicalResource(resource)
	}

	method, path, body, mutation, err := buildRequest(verb, resource, id, requestOptions{
		file: *file, username: *username, password: *password, moduleID: *moduleID,
		children: *children, permissions: *permissions, page: *page, size: *size,
	})
	if err != nil {
		return err
	}
	if mutation && *key == "" {
		*key = mutationKey()
	}
	if *wait && !mutation {
		return fmt.Errorf("--wait is only valid for mutations")
	}

	cli := api.New(*server, *token)
	env, err := cli.Do(context.Background(), method, path, body, *key)
	if err != nil {
		return err
	}
	result := env.Data
	if *wait {
		var op api.Operation
		if err := json.Unmarshal(env.Data, &op); err != nil {
			return err
		}
		// Synchronous CRUD responses are returned directly even when --wait is
		// present. Only accepted resource operations need polling.
		if op.ID != "" && op.State != "" {
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()
			done, err := cli.Wait(ctx, op.ID, 500*time.Millisecond)
			if err != nil {
				return err
			}
			result, _ = json.Marshal(done)
		}
	}
	return printJSON(out, result)
}

// normalizeFlags permits kubectl-style placement of global flags after the
// verb/resource (for example, `create modules --file module.json`) while
// retaining the standard flag package for validation and help output.
func normalizeFlags(args []string) []string {
	valueFlags := map[string]bool{
		"server": true, "token": true, "context": true, "output": true,
		"timeout": true, "idempotency-key": true, "file": true,
		"username": true, "password": true, "module-id": true,
		"page": true, "size": true,
	}
	boolFlags := map[string]bool{"wait": true, "children": true, "permissions": true}
	shortValueFlags := map[string]bool{"f": true, "o": true}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			positional = append(positional, arg)
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			nameValue := strings.TrimPrefix(arg, "-")
			name := nameValue
			hasValue := strings.Contains(nameValue, "=")
			if hasValue {
				name = strings.SplitN(nameValue, "=", 2)[0]
			}
			if !shortValueFlags[name] {
				positional = append(positional, arg)
				continue
			}
			flags = append(flags, arg)
			if !hasValue && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		nameValue := strings.TrimPrefix(arg, "--")
		name := nameValue
		hasValue := strings.Contains(nameValue, "=")
		if hasValue {
			name = strings.SplitN(nameValue, "=", 2)[0]
		}
		if !valueFlags[name] && !boolFlags[name] {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		if valueFlags[name] && !hasValue && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positional...)
}

func usageError() error {
	return fmt.Errorf("usage: kumctl [flags] <login|list|get|create|update|delete|adopt> <resource> [id]")
}

func parsePositionals(pos []string) (verb, resource, id string, err error) {
	if pos[0] == "login" {
		if len(pos) != 1 {
			return "", "", "", fmt.Errorf("login does not accept positional arguments")
		}
		return "login", "login", "", nil
	}
	if pos[0] == "auth" {
		if len(pos) < 2 {
			return "", "", "", fmt.Errorf("auth requires login")
		}
		if len(pos) > 3 {
			return "", "", "", fmt.Errorf("auth accepts one operation")
		}
		return "auth", pos[1], first(pos, 2), nil
	}
	if len(pos) < 2 {
		return "", "", "", usageError()
	}
	if len(pos) > 3 {
		return "", "", "", fmt.Errorf("too many positional arguments")
	}
	for _, value := range pos {
		if strings.HasPrefix(value, "-") {
			return "", "", "", fmt.Errorf("unexpected positional argument %q", value)
		}
	}
	return pos[0], pos[1], first(pos, 2), nil
}

func first(pos []string, index int) string {
	if len(pos) > index {
		return pos[index]
	}
	return ""
}

func canonicalResource(resource string) string {
	return map[string]string{
		"user": "users", "users": "users", "role": "roles", "roles": "roles",
		"module": "modules", "modules": "modules", "project": "projects", "projects": "projects",
		"workspace": "workspaces", "workspaces": "workspaces", "application": "applications", "applications": "applications",
		"cluster": "clusters", "clusters": "clusters", "operation": "operations", "operations": "operations",
	}[resource]
}

type requestOptions struct {
	file, username, password, moduleID string
	children, permissions              bool
	page, size                         int
}

func buildRequest(verb, resource, id string, opts requestOptions) (method, path string, body []byte, mutation bool, err error) {
	if verb == "login" || (verb == "auth" && resource == "login") {
		if opts.file != "" {
			body, err = os.ReadFile(opts.file)
		} else if opts.username == "" || opts.password == "" {
			err = fmt.Errorf("login requires --username and --password or --file")
		} else {
			body, err = json.Marshal(loginRequest{Username: opts.username, Password: opts.password})
		}
		return http.MethodPost, "/auth/login", body, false, err
	}
	if resource == "" {
		return "", "", nil, false, fmt.Errorf("unsupported resource")
	}
	if verb == "list" {
		if id != "" {
			return "", "", nil, false, fmt.Errorf("list does not accept an id")
		}
		if resource == "operations" {
			return "", "", nil, false, fmt.Errorf("list is not supported for operations; use get operations <id>")
		}
		if opts.children || opts.permissions {
			return "", "", nil, false, fmt.Errorf("--children/--permissions require get with an id")
		}
		path = "/" + resource
		if resource == "projects" && opts.moduleID != "" {
			path = "/projects/module/" + url.PathEscape(opts.moduleID)
		} else if opts.moduleID != "" {
			return "", "", nil, false, fmt.Errorf("--module-id is only valid for list projects")
		}
		return http.MethodGet, addPagination(path, opts.page, opts.size), nil, false, nil
	}
	if verb == "get" {
		if id == "" {
			return "", "", nil, false, fmt.Errorf("get requires an id")
		}
		if opts.children {
			if resource != "modules" || opts.permissions {
				return "", "", nil, false, fmt.Errorf("--children is only valid for get modules <id>")
			}
			return http.MethodGet, "/modules/" + url.PathEscape(id) + "/children", nil, false, nil
		}
		if opts.permissions {
			if resource != "roles" {
				return "", "", nil, false, fmt.Errorf("--permissions is only valid for get roles <id>")
			}
			return http.MethodGet, "/roles/" + url.PathEscape(id) + "/permissions", nil, false, nil
		}
		return http.MethodGet, "/" + resource + "/" + url.PathEscape(id), nil, false, nil
	}
	if id == "" && (verb == "update" || verb == "delete" || verb == "adopt") {
		return "", "", nil, false, fmt.Errorf("%s requires an id", verb)
	}
	switch verb {
	case "create":
		if !contains([]string{"users", "modules", "projects", "workspaces", "applications"}, resource) {
			return "", "", nil, false, fmt.Errorf("create is not supported for %s", resource)
		}
		body, err = readBody(opts.file, "create")
		return http.MethodPost, "/" + resource, body, true, err
	case "update":
		if !contains([]string{"users", "modules", "projects", "workspaces", "applications"}, resource) {
			return "", "", nil, false, fmt.Errorf("update is not supported for %s", resource)
		}
		body, err = readBody(opts.file, "update")
		return http.MethodPut, "/" + resource + "/" + url.PathEscape(id), body, true, err
	case "delete":
		if !contains([]string{"users", "modules", "projects", "workspaces", "applications"}, resource) {
			return "", "", nil, false, fmt.Errorf("delete is not supported for %s", resource)
		}
		return http.MethodDelete, "/" + resource + "/" + url.PathEscape(id), nil, true, nil
	case "adopt":
		switch resource {
		case "clusters":
			return http.MethodPost, "/clusters/" + url.PathEscape(id) + "/adopt", nil, true, nil
		case "workspaces", "applications":
			body, err = readBody(opts.file, "adopt "+resource)
			return http.MethodPost, "/" + resource + "/" + url.PathEscape(id) + "/adopt-existing", body, true, err
		default:
			return "", "", nil, false, fmt.Errorf("adopt is only supported for clusters, workspaces, and applications")
		}
	default:
		return "", "", nil, false, fmt.Errorf("unsupported verb %q", verb)
	}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func readBody(file, operation string) ([]byte, error) {
	if file == "" {
		return nil, fmt.Errorf("%s requires --file", operation)
	}
	return os.ReadFile(file)
}

func addPagination(path string, page, size int) string {
	query := url.Values{}
	if page > 0 {
		query.Set("page", strconv.Itoa(page))
	}
	if size > 0 {
		query.Set("size", strconv.Itoa(size))
	}
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func printJSON(out io.Writer, result []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, result, "", "  "); err != nil {
		_, err = out.Write(result)
		return err
	}
	_, err := fmt.Fprintln(out, pretty.String())
	return err
}
