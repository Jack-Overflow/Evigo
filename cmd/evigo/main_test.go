package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGivenExampleConfigWhenValidatedThenNoNetworkOrDatabaseIsNeeded(t *testing.T) {
	// Given the synthetic example configuration and a nonexistent state directory.
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, data, 0o600)
	var stdout bytes.Buffer

	// When the offline validation command runs.
	if err := execute([]string{"validate", "--config", path}, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	// Then it reports valid configuration without requiring a database.
	if stdout.String() != "Configuration valid\n" {
		t.Fatal(stdout.String())
	}

	// When the version command runs.
	stdout.Reset()
	if err := execute([]string{"version"}, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	// When malformed commands are supplied.
	for _, arguments := range [][]string{nil, {"unknown"}, {"version", "bad"}, {"validate", "extra"}, {"state"}, {"validate", "--state", "x"}} {
		// Then each command is rejected.
		if execute(arguments, &stdout, &stdout) == nil {
			t.Fatal("invalid CLI accepted")
		}
	}
}

func TestGivenUnusableMACWhenValidatedThenValidationFails(t *testing.T) {
	// Given the example configuration with a broadcast MAC.
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, bytes.Replace(data, []byte("'02:00:00:00:00:10'"), []byte("'ff:ff:ff:ff:ff:ff'"), 1), 0o600)
	var output bytes.Buffer

	// When the validation command runs.
	err = execute([]string{"validate", "--config", path}, &output, &output)

	// Then the device's MAC is rejected.
	if err == nil {
		t.Fatal("accepted a broadcast MAC")
	}
}
