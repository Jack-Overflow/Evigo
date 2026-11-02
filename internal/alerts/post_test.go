package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const secretPath = "/hooks/secret-token"

func TestGivenServerAcceptsWhenJSONIsPostedThenItReceivesTheBodyAndHeaders(t *testing.T) {
	// Given a server that accepts requests.
	var received map[string]string
	var contentType, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		contentType = request.Header.Get("Content-Type")
		authorization = request.Header.Get("Authorization")
		json.NewDecoder(request.Body).Decode(&received)
	}))
	defer server.Close()

	// When JSON is posted with an extra header.
	header := http.Header{"Authorization": {"Bearer token"}}
	err := PostJSON(context.Background(), server.URL+secretPath, header, map[string]string{"title": "UPS on battery"})
	// Then the server receives the JSON body and both headers.
	if err != nil {
		t.Fatal(err)
	}
	if received["title"] != "UPS on battery" || contentType != "application/json" || authorization != "Bearer token" {
		t.Fatalf("body %v, content type %q, authorization %q", received, contentType, authorization)
	}
}

func TestGivenServerRejectsWhenJSONIsPostedThenTheErrorGivesOnlyTheStatus(t *testing.T) {
	// Given a server that rejects requests with a reply that repeats the secret.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, `{"message": "Unknown token secret-token"}`, http.StatusNotFound)
	}))
	defer server.Close()

	// When JSON is posted.
	err := PostJSON(context.Background(), server.URL+secretPath, nil, map[string]string{})

	// Then the error gives the status code and nothing from the URL or the reply.
	if err == nil || err.Error() != "server returned status 404" {
		t.Fatalf("error %v", err)
	}
}

func TestGivenUnreachableServerWhenJSONIsPostedThenTheErrorHidesTheURL(t *testing.T) {
	// Given a server that can't be reached.
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	// When JSON is posted.
	err := PostJSON(context.Background(), server.URL+secretPath, nil, map[string]string{})

	// Then the error never contains the secret.
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error %v", err)
	}
}

func TestGivenSilentServerWhenTheDeadlinePassesThenPostReturns(t *testing.T) {
	// Given a server that never replies.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	// When JSON is posted with a short deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := PostJSON(ctx, server.URL+secretPath, nil, map[string]string{})

	// Then it gives up at the deadline without exposing the secret.
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("returned after %s", elapsed)
	}
}
