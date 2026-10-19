package nut

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
)

const healthyResponse = "BEGIN LIST VAR main\nVAR main ups.status \"OL\"\nVAR main battery.charge \"100\"\nEND LIST VAR main\n"

func TestGivenFailedReadWhenReadingAgainThenReaderReconnects(t *testing.T) {
	// Given a server whose first connection fails mid-response and later ones answer.
	responses := []string{"BEGIN LIST VAR main\n", healthyResponse, healthyResponse}
	dials := 0
	dial := func(context.Context, string, string) (net.Conn, error) {
		clientConnection, serverConnection := net.Pipe()
		response := responses[dials]
		dials++
		go func() {
			defer serverConnection.Close()
			lines := bufio.NewReader(serverConnection)
			for {
				if _, err := lines.ReadString('\n'); err != nil {
					return
				}
				fmt.Fprint(serverConnection, response)
				if response != healthyResponse {
					return
				}
			}
		}()

		return clientConnection, nil
	}
	reader := NewReader(settings(), dial)
	defer reader.Close()

	// When the first read fails and two more reads follow.
	_, firstErr := reader.Read(context.Background())
	second, secondErr := reader.Read(context.Background())
	_, thirdErr := reader.Read(context.Background())

	// Then the failed connection is replaced once and then reused.
	if firstErr == nil || secondErr != nil || thirdErr != nil || second.Status != "OL" {
		t.Fatalf("errors %v %v %v, reading %+v", firstErr, secondErr, thirdErr, second)
	}
	if dials != 2 {
		t.Fatalf("dialled %d times", dials)
	}
}
