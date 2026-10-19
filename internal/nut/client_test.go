package nut

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

func settings() Settings {
	return Settings{
		Host: "127.0.0.1", Port: 3493, UPS: "main",
		ReadTimeout: 50 * time.Millisecond, ConnectTimeout: 50 * time.Millisecond,
	}
}

func pipeClient(t *testing.T, response string) *Client {
	t.Helper()
	clientConnection, serverConnection := net.Pipe()
	currentConfig := settings()
	go func() {
		defer serverConnection.Close()
		bufio.NewReader(serverConnection).ReadString('\n')
		fmt.Fprint(serverConnection, response)
	}()
	return &Client{clientConnection, bufio.NewReaderSize(clientConnection, 16*1024), currentConfig}
}

func TestGivenReadingWhenCopiedThenValuesAndMetadataAreIndependent(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		values map[string]string
	}{
		{name: "nil values"},
		{name: "empty values", values: map[string]string{}},
		{name: "populated values", values: map[string]string{"ups.status": "OL"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a reading with metadata and optional raw values.
			original := Reading{At: time.Now(), Status: "OL", Battery: 80, Values: scenario.values}

			// When a separate reading is copied from it.
			copied := original.Copy()

			// Then metadata is preserved and the copy always has a writable values map.
			if copied.At != original.At || copied.Status != original.Status || copied.Battery != original.Battery || !maps.Equal(copied.Values, original.Values) {
				t.Fatal("reading copy lost metadata or raw values")
			}

			if copied.Values == nil {
				t.Fatal("reading copy has no writable values map")
			}

			// When the caller changes the copied reading.
			copied.Status = "OB"
			copied.Values["copy-only"] = "changed"

			// Then the original reading is unaffected.
			if original.Status != "OL" || original.Values["copy-only"] != "" {
				t.Fatal("changing the copy affected the original")
			}

			if original.Values != nil {
				// When the original raw values change.
				original.Values["original-only"] = "changed"

				// Then the copy is unaffected.
				if copied.Values["original-only"] != "" {
					t.Fatal("changing the original affected the copy")
				}
			}
		})
	}
}

func TestGivenNUTResponseWhenReadThenValuesParseAndStatusFlagsDecideSafety(t *testing.T) {
	// Given a NUT response containing charge, power status, and escaped model text.
	response := "BEGIN LIST VAR main\nVAR main ups.status \"OL CHRG\"\nVAR main battery.charge \"25.5\"\nVAR main ups.model \"A \\\"quoted\\\" \\\\ model\"\nEND LIST VAR main\n"
	client := pipeClient(t, response)
	defer client.Close()

	// When the client reads the response.
	reading, err := client.Read(context.Background())
	// Then parsed values and charge thresholds are correct.
	if err != nil {
		t.Fatal(err)
	}
	if !reading.Safe(25) || reading.Safe(26) || reading.Values["ups.model"] != `A "quoted" \ model` {
		t.Fatalf("bad parsed reading: %+v", reading)
	}

	// When status contains unsafe, contradictory, or unknown flags.
	for _, status := range []string{"OB", "OL OB", "OL LB", "OL FSD", "OL OFF", "OL ALARM", "OL BYPASS", "OL UNKNOWN", "NOTOL", ""} {
		reading.Status = status

		// Then waking is unsafe.
		if reading.Safe(0) {
			t.Fatalf("unsafe status accepted: %s", status)
		}
	}

	// When the status reports battery operation.
	reading.Status = "OB LB"

	// Then an outage is detected.
	if !reading.OnBattery() {
		t.Fatal("battery event not detected")
	}

	// When status contradicts itself.
	reading.Status = "OL OB"

	// Then it cannot establish an outage.
	if reading.OnBattery() {
		t.Fatal("contradiction treated as event")
	}
}

func TestGivenMalformedOrUnavailableResponseWhenReadThenItIsRejected(t *testing.T) {
	// Given malformed, incomplete, stale, and unavailable NUT responses.
	for name, response := range map[string]string{
		"stale": "ERR DATA-STALE\n", "driver": "ERR DRIVER-NOT-CONNECTED\n",
		"partial":   "BEGIN LIST VAR main\nVAR main ups.status \"OL\"\n",
		"missing":   "BEGIN LIST VAR main\nVAR main ups.status \"OL\"\nEND LIST VAR main\n",
		"nan":       "BEGIN LIST VAR main\nVAR main ups.status \"OL\"\nVAR main battery.charge \"NaN\"\nEND LIST VAR main\n",
		"wrong UPS": "BEGIN LIST VAR other\n", "bad quote": "BEGIN LIST VAR main\nVAR main ups.status \"OL\n",
		"oversized": "BEGIN LIST VAR main\n" + strings.Repeat("x", 20000) + "\n",
		"duplicate": "BEGIN LIST VAR main\nVAR main battery.charge \"30\"\nVAR main battery.charge \"40\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			// Given a client receiving this invalid response.
			client := pipeClient(t, response)
			defer client.Close()

			// When it reads the response.
			_, err := client.Read(context.Background())

			// Then the invalid reading is rejected.
			if err == nil {
				t.Fatal("invalid read accepted")
			}
		})
	}
}

func TestGivenStalledServerWhenReadIsCancelledOrTimesOutThenItFailsPromptly(t *testing.T) {
	// Given a server that never completes a response and optional immediate cancellation.
	for _, cancelImmediately := range []bool{false, true} {
		clientConnection, serverConnection := net.Pipe()
		go func() {
			defer serverConnection.Close()
			bufio.NewReader(serverConnection).ReadString('\n')
			bufio.NewReader(serverConnection).ReadString('\n')
		}()
		client := &Client{clientConnection, bufio.NewReader(clientConnection), settings()}
		ctx, cancel := context.WithCancel(context.Background())
		if cancelImmediately {
			cancel()
		}
		start := time.Now()

		// When the client reads with a bounded context.
		_, err := client.Read(ctx)
		cancel()
		client.Close()

		// Then the read fails promptly on cancellation or timeout.
		if err == nil || time.Since(start) > time.Second {
			t.Fatal("read did not stop at deadline/cancellation")
		}
	}
}

func TestGivenSpecialCharacterCredentialsWhenAuthenticatingThenTheyAreEscapedAndNeverLeaked(t *testing.T) {
	// Given credentials containing quotes and backslashes and a recording server.
	currentConfig := settings()
	currentConfig.Username = "monitor"
	currentConfig.Password = `secret " \`
	commands := make(chan []string, 1)
	dial := func(context.Context, string, string) (net.Conn, error) {
		clientConnection, serverConnection := net.Pipe()
		go func() {
			defer serverConnection.Close()
			commandReader := bufio.NewReader(serverConnection)
			var receivedCommands []string
			for commandIndex := 0; commandIndex < 2; commandIndex++ {
				line, _ := commandReader.ReadString('\n')
				parts, _ := parseLine(strings.TrimSpace(line))
				receivedCommands = append(receivedCommands, parts[1])
				fmt.Fprint(serverConnection, "OK\n")
			}
			commands <- receivedCommands
		}()
		return clientConnection, nil
	}

	// When authentication succeeds.
	client, err := Connect(context.Background(), currentConfig, dial)
	// Then the server receives the original credentials.
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	receivedCommands := <-commands
	if receivedCommands[0] != currentConfig.Username || receivedCommands[1] != currentConfig.Password {
		t.Fatal("credentials not escaped correctly")
	}

	// Given a server that echoes the secret in an authentication error.
	rejectingDialer := func(context.Context, string, string) (net.Conn, error) {
		clientConnection, serverConnection := net.Pipe()
		go func() {
			defer serverConnection.Close()
			bufio.NewReader(serverConnection).ReadString('\n')
			fmt.Fprintln(serverConnection, "ERR", currentConfig.Password)
		}()
		return clientConnection, nil
	}

	// When the client authenticates against the rejecting server.
	_, err = Connect(context.Background(), currentConfig, rejectingDialer)

	// Then authentication fails without revealing the secret.
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("authentication error leaked or accepted")
	}
}

func TestGivenQuotedProtocolLineWhenParsedThenTokensExcludeQuotesAndComments(t *testing.T) {
	// Given a protocol line with quoted tokens and a trailing comment.

	// When the protocol line is parsed.
	parts, err := parseLine(`VAR "main" ups.model "one"two # comment`)

	// Then tokens exclude quotes and comments.
	if err != nil || strings.Join(parts, "|") != "VAR|main|ups.model|one|two" {
		t.Fatalf("bad tokens: %v %v", parts, err)
	}
}

func TestGivenTransportFailureWhenReportedThenOnlyTheCategoryIsShown(t *testing.T) {
	// Given transport failures wrapped with private diagnostic text.
	cases := []struct {
		err  error
		want string
	}{
		{io.EOF, "server closed the connection"},
		{net.ErrClosed, "connection closed locally"},
		{syscall.ECONNRESET, "connection reset by peer"},
		{syscall.ECONNREFUSED, "connection refused"},
		{syscall.EPIPE, "broken pipe"},
		{context.DeadlineExceeded, "timed out after 5s"},
		{context.Canceled, "cancelled"},
		{&net.DNSError{Err: "private-value", Name: "private-value"}, "DNS resolution failed"},
		{errors.New("private-value"), "I/O failed"},
	}
	for _, scenario := range cases {

		// When each failure is formatted for reporting.
		errorMessage := transportError("read", fmt.Errorf("private-value: %w", scenario.err), 5*time.Second).Error()

		// Then the category is useful without exposing private details.
		if !strings.Contains(errorMessage, scenario.want) || strings.Contains(errorMessage, "private-value") {
			t.Fatalf("incorrect or unsafe error: %s", errorMessage)
		}
	}
}

func TestGivenIncompleteResponseWhenReadFailsThenErrorReportsStageAndProgress(t *testing.T) {
	// Given a server closing after one variable in an incomplete list.
	client := pipeClient(t, "BEGIN LIST VAR main\nVAR main ups.status \"OL\"\n")
	defer client.Close()

	// When the client reads the partial response.
	_, err := client.Read(context.Background())

	// Then the error identifies the variable stage and progress.
	if err == nil || !strings.Contains(err.Error(), "server closed the connection") || !strings.Contains(err.Error(), "stage=variables, variables=1") {
		t.Fatalf("missing read diagnostics: %v", err)
	}

	// Given a server that never sends a list header.
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()
	go func() { bufio.NewReader(serverConnection).ReadString('\n') }()
	client = &Client{clientConnection, bufio.NewReader(clientConnection), settings()}

	// When the header read times out.
	_, err = client.Read(context.Background())

	// Then the error reports the header stage and zero variables.
	if err == nil || !strings.Contains(err.Error(), "timed out after 50ms") || !strings.Contains(err.Error(), "stage=list_header, variables=0") {
		t.Fatalf("missing timeout diagnostics: %v", err)
	}
}

func TestGivenUntrustedStatusWhenFormattedForLogsThenSafetyStillUsesOriginalFlags(t *testing.T) {
	// Given
	reading := Reading{Status: "OL secret-text CHRG secret-text", Battery: 100}

	// When
	status := reading.LogStatus()

	// Then
	if status != "OL UNKNOWN CHRG" || reading.Safe(25) || reading.Status != "OL secret-text CHRG secret-text" {
		t.Fatal("status formatting exposed raw text or changed safety decisions")
	}
}
