package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const minimal = `nut:
  host: localhost
  ups: main
devices:
  - id: server
    name: Server
    host: 192.0.2.10
    mac: "02:00:00:00:00:10"
`

func loadText(t *testing.T, text string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestGivenEnvironmentCredentialsWhenLoadedThenSecretsExpandAndDefaultsApply(t *testing.T) {
	// Given a minimal configuration with credentials supplied by the environment.
	t.Setenv("EVIGO_TEST_USER", "monitor")
	t.Setenv("EVIGO_TEST_PASSWORD", "a: # ' \" complicated secret")

	// When the configuration is loaded.
	currentConfig, err := loadText(t, strings.Replace(minimal, "  ups: main", "  ups: main\n  username: ${EVIGO_TEST_USER}\n  password: ${EVIGO_TEST_PASSWORD}", 1))
	// Then credentials are expanded literally and omitted settings receive defaults.
	if err != nil {
		t.Fatal(err)
	}
	if currentConfig.NUT.Password != os.Getenv("EVIGO_TEST_PASSWORD") || currentConfig.Poll.Interval != 10*time.Second || currentConfig.WOL.Port != 9 {
		t.Fatalf("defaults or credential expansion failed")
	}
}

func TestGivenInvalidConfigurationWhenLoadedThenItFailsWithoutLeakingSecrets(t *testing.T) {
	// Given invalid configuration variants and a value that must remain private.
	cases := map[string]string{
		"unknown":             minimal + "unknown: true\n",
		"nested unknown":      strings.Replace(minimal, "  ups: main", "  ups: main\n  hostname: localhost", 1),
		"type error":          minimal + "probe: {timeout: secret-value}\n",
		"duplicate YAML key":  minimal + "nut: {host: other}\n",
		"extra document":      minimal + "---\n{}\n",
		"unresolved secret":   strings.Replace(minimal, "  ups: main", "  ups: main\n  username: monitor\n  password: ${EVIGO_TEST_MISSING}", 1),
		"partial auth":        strings.Replace(minimal, "  ups: main", "  ups: main\n  username: monitor", 1),
		"bad threshold":       minimal + "recovery: {min_battery_percent: 101}\n",
		"nan threshold":       minimal + "recovery: {min_battery_percent: .nan}\n",
		"negative duration":   minimal + "probe: {timeout: -2s}\n",
		"stale interval":      minimal + "poll: {interval: 40s}\n",
		"zero port":           strings.Replace(minimal, "  ups: main", "  ups: main\n  port: 0", 1),
		"missing MAC":         strings.Replace(minimal, "    mac: \"02:00:00:00:00:10\"\n", "", 1),
		"bad broadcast":       minimal + "wol: {broadcast: broadcast.example}\n",
		"duplicate ID":        minimal + "  - {id: server, name: other, host: localhost, mac: '02:00:00:00:00:11'}\n",
		"unknown section":     minimal + "dashboard: {listen: 127.0.0.1:8080}\n",
		"both passwords":      strings.Replace(minimal, "  ups: main", "  ups: main\n  password: secret-value\n  password_file: /tmp/password", 1),
		"email unknown key":   minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan, to: [b@example.lan], smtp_host: x}}\n",
		"email no host":       minimal + "alerts: {email: {from: a@example.lan, to: [b@example.lan]}}\n",
		"email no from":       minimal + "alerts: {email: {host: smtp.example.lan, to: [b@example.lan]}}\n",
		"email no to":         minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan}}\n",
		"email named from":    minimal + "alerts: {email: {host: smtp.example.lan, from: 'Evigo <a@example.lan>', to: [b@example.lan]}}\n",
		"email bad to":        minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan, to: [secret-value]}}\n",
		"email bad security":  minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan, to: [b@example.lan], security: ssl}}\n",
		"email partial auth":  minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan, to: [b@example.lan], username: monitor}}\n",
		"email plain login":   minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan, to: [b@example.lan], security: none, username: monitor, password: secret-value}}\n",
		"email two passwords": minimal + "alerts: {email: {host: smtp.example.lan, from: a@example.lan, to: [b@example.lan], username: monitor, password: secret-value, password_file: /tmp/password}}\n",
		"email bad port":      minimal + "alerts: {email: {host: smtp.example.lan, port: 70000, from: a@example.lan, to: [b@example.lan]}}\n",
		"discord plain http":  minimal + "alerts: {discord: {webhook_url: 'http://discord.com/api/webhooks/1/secret-value'}}\n",
		"discord other host":  minimal + "alerts: {discord: {webhook_url: 'https://example.lan/api/webhooks/1/secret-value'}}\n",
		"discord other path":  minimal + "alerts: {discord: {webhook_url: 'https://discord.com/channels/1/secret-value'}}\n",
		"discord unresolved":  minimal + "alerts: {discord: {webhook_url: '${EVIGO_TEST_MISSING}'}}\n",
		"ntfy bad topic":      minimal + "alerts: {ntfy: {topic: 'secret-value/other'}}\n",
		"ntfy bad url":        minimal + "alerts: {ntfy: {url: 'ftp://ntfy.example.lan', topic: alerts}}\n",
		"ntfy plain token":    minimal + "alerts: {ntfy: {url: 'http://ntfy.example.lan', topic: alerts, token: secret-value}}\n",
		"ntfy token no topic": minimal + "alerts: {ntfy: {token: secret-value}}\n",
		"webhook bad scheme":  minimal + "alerts: {webhook: {url: 'ftp://example.lan/secret-value'}}\n",
		"webhook no host":     minimal + "alerts: {webhook: {url: 'https:///secret-value'}}\n",
		"webhook unresolved":  minimal + "alerts: {webhook: {url: '${EVIGO_TEST_MISSING}'}}\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			// When each invalid configuration is loaded.
			_, err := loadText(t, text)

			// Then loading fails without exposing the private value.
			if err == nil {
				t.Fatal("accepted invalid config")
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatal("secret leaked in error")
			}
		})
	}
}

func TestGivenPasswordFileAndPortOverrideWhenLoadedThenBothAreApplied(t *testing.T) {
	// Given a password file and an explicit WOL port.
	path := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(path, []byte("a-secret\n"), 0o600)
	text := strings.Replace(minimal, "  ups: main", "  ups: main\n  username: monitor\n  password_file: "+path, 1)
	text += "wol: {port: 7}\n"

	// When the configuration is loaded.
	currentConfig, err := loadText(t, text)
	// Then the file secret and port override are used.
	if err != nil {
		t.Fatal(err)
	}
	if currentConfig.NUT.Password != "a-secret" || currentConfig.WOL.Port != 7 {
		t.Fatal("password file/default override failed")
	}
}

func TestGivenBadKeyOrValueWhenLoadedThenErrorNamesTheKeyAndLineButNotTheValue(t *testing.T) {
	for _, scenario := range []struct {
		name string
		text string
		want string
	}{
		{"unknown nested key", strings.Replace(minimal, "  ups: main", "  ups: main\n  reconnect: {min_delay: 1s}", 1), "unknown field nut.reconnect (line 4)"},
		{"unknown top-level key", minimal + "dashboard:\n  listen: 0.0.0.0:8080\n", "unknown field dashboard (line 9)"},
		{"duplicate key", minimal + "nut: {host: other}\n", "duplicate field nut (line 9)"},
		{"invalid duration", minimal + "probe:\n  timeout: secret-value\n", "invalid value for probe.timeout (line 10)"},
		{"unknown device field", strings.Replace(minimal, "    name: Server", "    name: Server\n    port: 9", 1), "unknown field devices[0].port (line 7)"},
		{"list where mapping expected", minimal + "wol: [secret-value]\n", "invalid value for wol (line 9)"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a configuration with one bad key or value.

			// When it is loaded.
			_, err := loadText(t, scenario.text)

			// Then the error names the key path and line, and never the value.
			if err == nil || err.Error() != scenario.want {
				t.Fatalf("got %v, want %q", err, scenario.want)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatal("value leaked in error")
			}
		})
	}
}

func TestGivenEmptyFileWhenLoadedThenItFailsCleanly(t *testing.T) {
	// Given an empty configuration file.

	// When it is loaded.
	_, err := loadText(t, "")

	// Then loading fails with an error instead of panicking.
	if err == nil {
		t.Fatal("accepted empty configuration")
	}
}

func TestGivenWebSettingsWhenValidatedThenOnlyAddressesAndExactHostsAreAccepted(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		web   Web
		valid bool
	}{
		{"disabled", Web{}, true},
		{"loopback", Web{Listen: "127.0.0.1:8080"}, true},
		{"all interfaces", Web{Listen: ":8080"}, true},
		{"IPv6", Web{Listen: "[::1]:8080"}, true},
		{"proxy hosts", Web{Listen: "127.0.0.1:8080", AllowedHosts: []string{"evigo.example.lan", "192.0.2.5"}}, true},
		{"port in host", Web{Listen: ":8080", AllowedHosts: []string{"evigo.example.lan:8443"}}, false},
		{"hostname listener", Web{Listen: "evigo.example.lan:8080"}, false},
		{"missing port", Web{Listen: "127.0.0.1"}, false},
		{"bad port", Web{Listen: "127.0.0.1:99999"}, false},
		{"scheme in host", Web{Listen: ":8080", AllowedHosts: []string{"https://evigo.example.lan"}}, false},
		{"wildcard host", Web{Listen: ":8080", AllowedHosts: []string{"*.example.lan"}}, false},
		{"path in host", Web{Listen: ":8080", AllowedHosts: []string{"evigo.example.lan/"}}, false},
		{"empty host", Web{Listen: ":8080", AllowedHosts: []string{""}}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a valid configuration with these web settings.
			settings, err := loadText(t, minimal)
			if err != nil {
				t.Fatal(err)
			}
			settings.Web = scenario.web

			// When it is validated.
			err = settings.Validate()

			// Then only IP listen addresses and exact host names are accepted.
			if (err == nil) != scenario.valid {
				t.Fatalf("validation error %v; want valid=%t", err, scenario.valid)
			}
		})
	}
}

func TestGivenNoAlertsSectionWhenLoadedThenEmailIsOff(t *testing.T) {
	// Given a configuration without an alerts section.

	// When it is loaded.
	currentConfig, err := loadText(t, minimal)
	// Then email is off and secure defaults are ready if it is turned on.
	if err != nil {
		t.Fatal(err)
	}
	if email := currentConfig.Alerts.Email; email.Host != "" || email.Port != 587 || email.Security != "starttls" {
		t.Fatalf("email %+v", email)
	}
	if currentConfig.Alerts.Discord.WebhookURL != "" {
		t.Fatal("discord is on")
	}
	if ntfy := currentConfig.Alerts.Ntfy; ntfy.Topic != "" || ntfy.URL != "https://ntfy.sh" {
		t.Fatalf("ntfy %+v", ntfy)
	}
	if currentConfig.Alerts.Webhook.URL != "" {
		t.Fatal("webhook is on")
	}
}

func TestGivenNtfyAndWebhookSecretsFromEnvironmentWhenLoadedThenTheyExpand(t *testing.T) {
	// Given ntfy and webhook settings with secrets supplied by the environment.
	t.Setenv("EVIGO_TEST_NTFY_TOKEN", "tk_secret")
	t.Setenv("EVIGO_TEST_WEBHOOK_URL", "https://home.example.lan/api/webhook/evigo-secret")
	text := minimal + `alerts:
  ntfy:
    topic: evigo-alerts
    token: "${EVIGO_TEST_NTFY_TOKEN}"
  webhook:
    url: "${EVIGO_TEST_WEBHOOK_URL}"
`

	// When the configuration is loaded.
	currentConfig, err := loadText(t, text)
	// Then the secrets are expanded and ntfy uses the public server.
	if err != nil {
		t.Fatal(err)
	}
	if ntfy := currentConfig.Alerts.Ntfy; ntfy.Token != "tk_secret" || ntfy.URL != "https://ntfy.sh" {
		t.Fatal("ntfy settings were not loaded")
	}
	if currentConfig.Alerts.Webhook.URL != "https://home.example.lan/api/webhook/evigo-secret" {
		t.Fatal("webhook URL was not expanded")
	}
}

func TestGivenSelfHostedNtfyWithoutTLSWhenNoTokenIsSetThenItIsAccepted(t *testing.T) {
	// Given a self-hosted ntfy server on the local network without TLS or a token.
	text := minimal + "alerts: {ntfy: {url: 'http://192.0.2.30:8090', topic: evigo}}\n"

	// When the configuration is loaded.
	_, err := loadText(t, text)
	// Then it is accepted, because nothing secret is sent.
	if err != nil {
		t.Fatal(err)
	}
}

func TestGivenDiscordWebhookFromEnvironmentWhenLoadedThenItExpands(t *testing.T) {
	// Given a Discord webhook supplied by the environment.
	t.Setenv("EVIGO_TEST_DISCORD_WEBHOOK", "https://discord.com/api/webhooks/123/webhook-secret")
	text := minimal + "alerts: {discord: {webhook_url: '${EVIGO_TEST_DISCORD_WEBHOOK}'}}\n"

	// When the configuration is loaded.
	currentConfig, err := loadText(t, text)
	// Then the webhook URL is expanded.
	if err != nil {
		t.Fatal(err)
	}
	if currentConfig.Alerts.Discord.WebhookURL != "https://discord.com/api/webhooks/123/webhook-secret" {
		t.Fatal("discord webhook was not expanded")
	}
}

func TestGivenEmailCredentialsFromEnvironmentWhenLoadedThenTheyExpand(t *testing.T) {
	// Given email settings with credentials supplied by the environment.
	t.Setenv("EVIGO_TEST_SMTP_USER", "monitor")
	t.Setenv("EVIGO_TEST_SMTP_PASSWORD", "smtp-secret")
	text := minimal + `alerts:
  email:
    host: smtp.example.lan
    username: ${EVIGO_TEST_SMTP_USER}
    password: ${EVIGO_TEST_SMTP_PASSWORD}
    from: evigo@example.lan
    to: [admin@example.lan]
`

	// When the configuration is loaded.
	currentConfig, err := loadText(t, text)
	// Then the credentials are expanded.
	if err != nil {
		t.Fatal(err)
	}
	if email := currentConfig.Alerts.Email; email.Username != "monitor" || email.Password != "smtp-secret" {
		t.Fatal("email credentials were not expanded")
	}
}

func TestGivenEmailPasswordFileWhenLoadedThenItIsUsed(t *testing.T) {
	// Given email settings with a password file.
	path := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(path, []byte("smtp-secret\n"), 0o600)
	text := minimal + "alerts: {email: {host: smtp.example.lan, username: monitor, password_file: " + path + ", from: a@example.lan, to: [b@example.lan]}}\n"

	// When the configuration is loaded.
	currentConfig, err := loadText(t, text)
	// Then the password comes from the file.
	if err != nil {
		t.Fatal(err)
	}
	if currentConfig.Alerts.Email.Password != "smtp-secret" {
		t.Fatal("email password file was not used")
	}
}

func TestGivenMissingEmailPasswordFileWhenLoadedThenTheErrorNamesTheEmailSetting(t *testing.T) {
	// Given email settings pointing at a missing password file.
	path := filepath.Join(t.TempDir(), "missing")
	text := minimal + "alerts: {email: {host: smtp.example.lan, username: monitor, password_file: " + path + ", from: a@example.lan, to: [b@example.lan]}}\n"

	// When the configuration is loaded.
	_, err := loadText(t, text)

	// Then the error names the email setting rather than the NUT one.
	if err == nil || err.Error() != "cannot open alerts.email.password_file" {
		t.Fatalf("error %v", err)
	}
}

func TestGivenRelayWithoutTLSWhenLoginIsToLocalhostThenItIsAccepted(t *testing.T) {
	// Given a local relay without TLS and with a login.
	text := minimal + "alerts: {email: {host: 127.0.0.1, port: 25, security: none, username: monitor, password: smtp-secret, from: a@example.lan, to: [b@example.lan]}}\n"

	// When the configuration is loaded.
	_, err := loadText(t, text)
	// Then it is accepted, because the login stays on this machine.
	if err != nil {
		t.Fatal(err)
	}
}
