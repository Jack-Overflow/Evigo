package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/nut"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
	"github.com/Jack-Overflow/Evigo/internal/snapshot"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type fakeSnapshots struct {
	latest  snapshot.Snapshot
	reading chan struct{}
	release chan struct{}
}

func (fake *fakeSnapshots) read() snapshot.Snapshot {
	if fake.release != nil {
		close(fake.reading)
		<-fake.release
	}

	return fake.latest
}

func testServer(listen string, allowedHosts ...string) (*Server, *fakeSnapshots) {
	mac, _ := net.ParseMAC("02:00:00:00:00:10")
	fake := &fakeSnapshots{latest: snapshot.Snapshot{
		CheckedAt: now.Add(-5 * time.Second),
		Reading:   &nut.Reading{At: now.Add(-5 * time.Second), Status: "OL CHRG", Battery: 80},
		SafeSince: now.Add(-time.Minute),
		StorageOK: true,
		RecoverySettings: recovery.Settings{
			StablePowerDelay:    30 * time.Second,
			ShutdownGracePeriod: 2 * time.Minute,
			MinBatteryPercent:   25,
			RetryInterval:       30 * time.Second,
			DeviceTimeout:       10 * time.Minute,
			MaxReadingAge:       30 * time.Second,
		},
		Devices: []snapshot.Device{
			{Device: devices.Device{ID: "server", Name: "Server", Host: "192.0.2.10", MAC: mac}, Reachability: devices.Online},
			{
				Device:       devices.Device{ID: "storage", Name: "Storage", Host: "192.0.2.11", MAC: mac},
				Reachability: devices.Offline,
				Pending:      &recovery.Progress{Attempts: 2, LastAttempt: now.Add(-10 * time.Second), TimeLeft: 9 * time.Minute},
				WakeSent:     true,
			},
		},
	}}
	server := New(Settings{
		Listen:        listen,
		AllowedHosts:  allowedHosts,
		Version:       "0.2.0",
		UPS:           "main-ups",
		HealthyWithin: time.Minute,
	}, fake.read)
	server.now = func() time.Time { return now }

	return server, fake
}

func get(server *Server, target, host string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = host
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	return recorder
}

func statusBody(t *testing.T, server *Server) statusResponse {
	t.Helper()
	response := get(server, "/api/status", "127.0.0.1:8080")
	var body statusResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	return body
}

func TestGivenRunningMonitorWhenStatusRequestedThenPowerDevicesAndSettingsAreReported(t *testing.T) {
	// Given
	server, _ := testServer("127.0.0.1:8080")

	// When
	body := statusBody(t, server)

	// Then
	power := body.Power
	if !power.Available || power.Block != nil || *power.BatteryPercent != 80 || strings.Join(power.Status, " ") != "OL CHRG" {
		t.Fatalf("power %+v", power)
	}
	if !power.WakeAllowedAt.Equal(now.Add(-30 * time.Second)) {
		t.Fatalf("wake allowed at %v", power.WakeAllowedAt)
	}
	if body.Devices[0].Recovery != nil || body.Devices[0].Reachability != "online" {
		t.Fatalf("idle device %+v", body.Devices[0])
	}
	pending := body.Devices[1].Recovery
	if pending == nil || pending.Attempts != 2 || pending.TimeLeftSeconds != 540 || !pending.WakeSent {
		t.Fatalf("pending device %+v", pending)
	}
	if body.Settings.StablePowerSeconds != 30 || body.Settings.MinBatteryPercent != 25 || body.Version != "0.2.0" {
		t.Fatalf("settings %+v", body.Settings)
	}
}

func TestGivenReadingOlderThanMaxAgeWhenStatusRequestedThenPowerIsReportedUnavailable(t *testing.T) {
	// Given
	server, fake := testServer("127.0.0.1:8080")
	fake.latest.Reading.At = now.Add(-time.Minute)

	// When
	body := statusBody(t, server)

	// Then
	if body.Power.Available || body.Power.Block == nil || *body.Power.Block != string(recovery.NoReading) {
		t.Fatalf("power %+v", body.Power)
	}
	if body.Power.StableSince != nil || body.Power.WakeAllowedAt != nil {
		t.Fatalf("expired reading kept a wake time: %+v", body.Power)
	}
}

func TestGivenSnapshotJustBeforeStabilityWhenTimePassesWithoutAPassThenStatusStopsBlocking(t *testing.T) {
	// Given a snapshot taken one second before power has been stable for the full delay.
	server, fake := testServer("127.0.0.1:8080")
	fake.latest.SafeSince = now.Add(-29 * time.Second)
	fake.latest.Reading.At = now

	// When status is read before and after the delay ends, with no new pass.
	before := statusBody(t, server)
	server.now = func() time.Time { return now.Add(2 * time.Second) }
	after := statusBody(t, server)

	// Then the block is worked out at request time.
	if before.Power.Block == nil || *before.Power.Block != string(recovery.Stabilising) {
		t.Fatalf("before %+v", before.Power)
	}
	if after.Power.Block != nil {
		t.Fatalf("after %+v", after.Power)
	}
}

func TestGivenListenerWhenRequestHostVariesThenOnlySafeHostsAreAccepted(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		listen  string
		allowed []string
		host    string
		want    bool
	}{
		{"loopback by IP", "127.0.0.1:8080", nil, "127.0.0.1:8080", true},
		{"loopback by localhost", "127.0.0.1:8080", nil, "localhost:8080", true},
		{"loopback refuses LAN IP", "127.0.0.1:8080", nil, "192.0.2.5:8080", false},
		{"loopback refuses DNS name", "127.0.0.1:8080", nil, "rebind.example:8080", false},
		{"LAN accepts IP", "0.0.0.0:8080", nil, "192.0.2.5:8080", true},
		{"LAN refuses DNS name", "0.0.0.0:8080", nil, "evigo.example.lan:8080", false},
		{"allowlist accepts exact name", "127.0.0.1:8080", []string{"evigo.example.lan"}, "EVIGO.example.lan", true},
		{"allowlist refuses others", "127.0.0.1:8080", []string{"evigo.example.lan"}, "127.0.0.1:8080", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given
			server, _ := testServer(scenario.listen, scenario.allowed...)

			// When
			response := get(server, "/api/status", scenario.host)

			// Then
			if (response.Code == http.StatusOK) != scenario.want {
				t.Fatalf("status %d", response.Code)
			}
		})
	}
}

func TestGivenCrossSiteOriginWhenStatusRequestedThenItIsRefused(t *testing.T) {
	// Given
	server, _ := testServer("127.0.0.1:8080")
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	request.Host = "127.0.0.1:8080"
	request.Header.Set("Origin", "https://attacker.example")

	// When
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	// Then
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status %d", recorder.Code)
	}
}

func TestGivenMonitorProgressWhenHealthCheckedThenStaleOrStartingLoopsAreUnhealthy(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		checkedAt time.Time
		wantCode  int
		wantBody  string
	}{
		{"recent pass", now.Add(-5 * time.Second), http.StatusOK, `"status":"ok"`},
		{"stalled loop", now.Add(-5 * time.Minute), http.StatusServiceUnavailable, `"status":"stalled"`},
		{"no pass yet", time.Time{}, http.StatusServiceUnavailable, `"status":"starting"`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given
			server, fake := testServer("127.0.0.1:8080")
			fake.latest.CheckedAt = scenario.checkedAt

			// When
			response := get(server, "/health", "127.0.0.1:8080")

			// Then
			if response.Code != scenario.wantCode || !strings.Contains(response.Body.String(), scenario.wantBody) {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
		})
	}
}

func TestGivenNoBuiltDashboardOrUnknownRouteWhenRequestedThenTheRightErrorIsReturned(t *testing.T) {
	// Given
	server, _ := testServer("127.0.0.1:8080")

	// When
	page := get(server, "/", "127.0.0.1:8080")
	unknownAPI := get(server, "/api/missing", "127.0.0.1:8080")
	request := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	request.Host = "127.0.0.1:8080"
	wrongMethod := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongMethod, request)

	// Then
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/status %d", wrongMethod.Code)
	}
	if page.Code != http.StatusServiceUnavailable || !strings.Contains(page.Body.String(), "make build") {
		t.Fatalf("page %d: %s", page.Code, page.Body)
	}
	if unknownAPI.Code != http.StatusNotFound || page.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("unknown API %d, headers %v", unknownAPI.Code, page.Header())
	}
}

func TestGivenRequestInProgressWhenServerStopsThenServeWaitsForTheResponse(t *testing.T) {
	// Given a running server with a status request still being handled.
	server, fake := testServer("127.0.0.1:0")
	fake.reading, fake.release = make(chan struct{}), make(chan struct{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, listener) }()
	responded := make(chan int, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/api/status")
		if err != nil {
			responded <- 0

			return
		}
		response.Body.Close()
		responded <- response.StatusCode
	}()
	<-fake.reading

	// When the server is told to stop while the handler is busy.
	stop()
	returnedEarly := false
	select {
	case <-served:
		returnedEarly = true
	case <-time.After(200 * time.Millisecond):
	}
	close(fake.release)

	// Then Serve waits for the handler, and the client gets its answer.
	if returnedEarly {
		t.Fatal("Serve returned while a request was still being handled")
	}
	if err = <-served; err != nil {
		t.Fatal(err)
	}
	if code := <-responded; code != http.StatusOK {
		t.Fatalf("client got %d", code)
	}
}
