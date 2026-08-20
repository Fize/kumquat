package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	return s
}
func mutationKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "kumctl-" + hex.EncodeToString(b)
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("kumctl", flag.ContinueOnError)
	fs.SetOutput(errOut)
	server := fs.String("server", os.Getenv("KUMQUAT_API_URL"), "Kumquat API base URL")
	token := fs.String("token", os.Getenv("KUMQUAT_TOKEN"), "bearer token")
	contextName := fs.String("context", "", "configured context")
	output := fs.String("output", "json", "output format (json)")
	wait := fs.Bool("wait", false, "wait for operation completion")
	timeout := fs.Duration("timeout", 2*time.Minute, "wait timeout")
	key := fs.String("idempotency-key", "", "mutation idempotency key")
	file := fs.String("file", "", "JSON request file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pos := fs.Args()
	if len(pos) < 2 {
		return fmt.Errorf("usage: kumctl [flags] <get|list|create|update|delete|adopt> <resource> [id]")
	}
	if *output != "json" {
		return fmt.Errorf("only --output json is supported")
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
	verb, resource := pos[0], pos[1]
	path := "/" + resource
	method := http.MethodGet
	var body []byte
	switch verb {
	case "list":
	case "get":
		if len(pos) < 3 {
			return fmt.Errorf("get requires an id")
		}
		path += "/" + pos[2]
	case "create":
		method = http.MethodPost
	case "update":
		if len(pos) < 3 {
			return fmt.Errorf("update requires an id")
		}
		method = http.MethodPut
		path += "/" + pos[2]
	case "delete":
		if len(pos) < 3 {
			return fmt.Errorf("delete requires an id")
		}
		method = http.MethodDelete
		path += "/" + pos[2]
	case "adopt":
		if resource != "clusters" || len(pos) < 3 {
			return fmt.Errorf("adopt requires clusters <id>")
		}
		method = http.MethodPost
		path += "/" + pos[2] + "/adopt"
	default:
		return fmt.Errorf("unsupported verb %q", verb)
	}
	if method == http.MethodPost || method == http.MethodPut {
		if *file != "" {
			var err error
			body, err = os.ReadFile(*file)
			if err != nil {
				return err
			}
		} else if verb != "adopt" {
			return fmt.Errorf("%s requires --file", verb)
		}
	}
	if method != http.MethodGet && *key == "" {
		*key = mutationKey()
	}
	cli := api.New(*server, *token)
	env, err := cli.Do(context.Background(), method, path, body, *key)
	if err != nil {
		return err
	}
	result := env.Data
	if *wait && method != http.MethodGet {
		var op api.Operation
		if err := json.Unmarshal(env.Data, &op); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		done, err := cli.Wait(ctx, op.ID, 500*time.Millisecond)
		if err != nil {
			return err
		}
		result, _ = json.Marshal(done)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, result, "", "  "); err != nil {
		_, err = out.Write(result)
		return err
	}
	fmt.Fprintln(out, pretty.String())
	return nil
}
