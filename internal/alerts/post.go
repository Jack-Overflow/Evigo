package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

func PostJSON(ctx context.Context, address string, header http.Header, payload any) error {
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid URL")
	}

	for name, values := range header {
		request.Header[name] = values
	}

	request.Header.Set("Content-Type", "application/json")
	// The client's error repeats the URL, which can hold a token or secret path.
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return errors.New("cannot reach the server")
	}

	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("server returned status %d", response.StatusCode)
	}

	return nil
}
