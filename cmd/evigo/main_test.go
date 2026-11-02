package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
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

func writeExampleConfig(t *testing.T, replace, with string) string {
	t.Helper()
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, bytes.Replace(data, []byte(replace), []byte(with), 1), 0o600)

	return path
}

func TestGivenWebhookConfiguredWhenAlertsAreTestedThenATestAlertIsSentAndReported(t *testing.T) {
	// Given the example configuration with a webhook on a loopback server.
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		json.NewDecoder(request.Body).Decode(&received)
	}))
	defer server.Close()
	path := writeExampleConfig(t, "  webhook:\n    url: ''", "  webhook:\n    url: '"+server.URL+"/hook'")
	var stdout bytes.Buffer

	// When the alerts are tested.
	err := execute([]string{"test-alerts", "--config", path}, &stdout, &stdout)
	// Then the webhook receives a test alert and the result is reported.
	if err != nil {
		t.Fatal(err)
	}
	if received["title"] != "Test alert" || stdout.String() != "webhook  sent\n" {
		t.Fatalf("received %v, output %q", received, stdout.String())
	}
}

func TestGivenNoProvidersWhenAlertsAreTestedThenItFails(t *testing.T) {
	// Given the example configuration, which turns every provider off.
	path := writeExampleConfig(t, "", "")
	var stdout bytes.Buffer

	// When the alerts are tested.
	err := execute([]string{"test-alerts", "--config", path}, &stdout, &stdout)

	// Then it says nothing is configured.
	if err == nil || err.Error() != "no alert providers are configured" {
		t.Fatalf("error %v", err)
	}
}

type fakeSender struct{ err error }

func (sender fakeSender) Send(context.Context, alerts.Alert) error { return sender.err }

func TestGivenOneProviderFailsWhenAlertsAreTestedThenEveryResultIsReportedAndItFails(t *testing.T) {
	// Given one working provider and one failing provider.
	senders := map[string]alerts.Sender{
		"ntfy":    fakeSender{err: errors.New("ntfy: server returned status 403")},
		"discord": fakeSender{},
	}
	var stdout bytes.Buffer

	// When the alerts are tested.
	err := testAlerts(senders, &stdout)

	// Then both results are reported in order and the command fails.
	if want := "discord  sent\nntfy     failed: ntfy: server returned status 403\n"; stdout.String() != want {
		t.Fatalf("output %q", stdout.String())
	}
	if err == nil || err.Error() != "1 of 2 alert providers failed" {
		t.Fatalf("error %v", err)
	}
}
