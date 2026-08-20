package client

import "testing"

func TestConfigCarriesExplicitKubeconfigInputs(t *testing.T) {
	cfg := Config{KubeconfigPath: "/tmp/test-kubeconfig", MasterURL: "https://hub.example.test"}
	if cfg.KubeconfigPath == "" || cfg.MasterURL == "" {
		t.Fatal("explicit Kubernetes client configuration was not retained")
	}
}
