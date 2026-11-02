package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

func TestGivenWebhookWhenAlertsAreSentThenEachArrivesAsJSONWithItsSeverity(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		alert alerts.Alert
		want  payload
	}{
		{
			"problem",
			alerts.OnBattery(time.Date(2026, 11, 2, 16, 30, 0, 0, time.FixedZone("BST", 3600)), 90),
			payload{Title: "UPS on battery", Body: "The UPS is running on battery at 90%.", Severity: "problem", At: "2026-11-02T15:30:00Z"},
		},
		{
			"resolved",
			alerts.DeviceRecovered(time.Date(2026, 11, 2, 16, 30, 0, 0, time.UTC), "Home server", 1),
			payload{Title: "Home server is back online", Body: "Home server answered ping after 1 wake attempt.", Severity: "resolved", At: "2026-11-02T16:30:00Z"},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a webhook that accepts requests.
			var received payload
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				json.NewDecoder(request.Body).Decode(&received)
			}))
			defer server.Close()

			// When the alert is sent.
			err := New(server.URL+"/api/webhook/evigo").Send(context.Background(), scenario.alert)
			// Then the webhook receives its title, body, severity and time.
			if err != nil {
				t.Fatal(err)
			}
			if received != scenario.want {
				t.Fatalf("payload %+v", received)
			}
		})
	}
}
