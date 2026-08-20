package main

import "testing"

func TestRemoteCredentialContractUsesMountedFiles(t *testing.T) {
	values := []string{bootstrapTokenFileEnv, hubCAFileEnv, tunnelCAFileEnv}
	for _, value := range values {
		if value == "" {
			t.Fatal("remote credential file environment name must not be empty")
		}
	}
}
