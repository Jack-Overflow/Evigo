package nut

import (
	"context"
)

type Reader struct {
	settings Settings
	dial     DialFunc
	client   *Client
}

func NewReader(settings Settings, dial DialFunc) *Reader {
	return &Reader{settings: settings, dial: dial}
}

func (reader *Reader) Read(ctx context.Context) (Reading, error) {
	if reader.client == nil {
		client, err := Connect(ctx, reader.settings, reader.dial)
		if err != nil {
			return Reading{}, err
		}

		reader.client = client
	}

	reading, err := reader.client.Read(ctx)
	if err != nil {
		reader.Close()
	}

	return reading, err
}

func (reader *Reader) Close() {
	if reader.client != nil {
		reader.client.Close()
		reader.client = nil
	}
}
