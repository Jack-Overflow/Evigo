package nut

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxLineBytes = 16 * 1024
	maxVariables = 4096
)

var decimal = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

type DialFunc func(context.Context, string, string) (net.Conn, error)

type Settings struct {
	Host           string
	Port           int
	UPS            string
	Username       string
	Password       string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
}

type Client struct {
	connection net.Conn
	reader     *bufio.Reader
	settings   Settings
}

func Connect(ctx context.Context, settings Settings, dial DialFunc) (*Client, error) {
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}

	dialCtx, cancel := context.WithTimeout(ctx, settings.ConnectTimeout)
	defer cancel()
	connection, err := dial(dialCtx, "tcp", net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port)))
	if err != nil {
		return nil, transportError("connect", err, settings.ConnectTimeout)
	}

	client := &Client{connection, bufio.NewReaderSize(connection, maxLineBytes), settings}
	if settings.Username == "" {
		return client, nil
	}

	if err = client.authenticate(ctx); err != nil {
		connection.Close()

		return nil, err
	}

	return client, nil
}

func (client *Client) Close() error {
	return client.connection.Close()
}

func (client *Client) authenticate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, client.settings.ReadTimeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { client.connection.Close() })
	defer stop()
	client.connection.SetDeadline(time.Now().Add(client.settings.ReadTimeout))
	for _, command := range []string{"USERNAME " + quote(client.settings.Username), "PASSWORD " + quote(client.settings.Password)} {
		if err := client.write(command); err != nil {
			return errors.New("NUT authentication failed")
		}

		parts, err := client.readLine()
		if err != nil || len(parts) != 1 || parts[0] != "OK" {
			return errors.New("NUT authentication failed")
		}
	}

	return nil
}

func (client *Client) Read(ctx context.Context) (reading Reading, err error) {
	readCtx, cancel := context.WithTimeout(ctx, client.settings.ReadTimeout)
	defer cancel()
	stop := context.AfterFunc(readCtx, func() { client.connection.Close() })
	defer stop()
	progress := readProgress{stage: "request"}
	defer func() {
		if err != nil {
			err = progress.describe(err, ctx, readCtx, client.settings.ReadTimeout)
		}
	}()

	client.connection.SetDeadline(time.Now().Add(client.settings.ReadTimeout))
	if err = client.write("LIST VAR " + quote(client.settings.UPS)); err != nil {
		return reading, err
	}

	values, err := client.readVariables(&progress)
	if err != nil {
		return reading, err
	}

	return readingFrom(values)
}

type readProgress struct {
	stage     string
	variables int
}

func (progress readProgress) describe(err error, ctx, readCtx context.Context, timeout time.Duration) error {
	switch {
	case ctx.Err() != nil:
		err = errors.New("NUT read cancelled")
	case readCtx.Err() != nil:
		err = fmt.Errorf("NUT read timed out after %s", timeout)
	}

	return fmt.Errorf("%w (stage=%s, variables=%d)", err, progress.stage, progress.variables)
}

func (client *Client) readVariables(progress *readProgress) (map[string]string, error) {
	progress.stage = "list_header"
	parts, err := client.readLine()
	if err != nil {
		return nil, err
	}

	if !client.listMarker(parts, "BEGIN") {
		return nil, errors.New("invalid NUT list header")
	}

	progress.stage = "variables"
	values := map[string]string{}
	for range maxVariables {
		parts, err = client.readLine()
		switch {
		case err != nil:
			return nil, err
		case client.listMarker(parts, "END"):
			return values, nil
		case len(parts) != 4 || parts[0] != "VAR" || parts[1] != client.settings.UPS:
			return nil, errors.New("invalid NUT variable response")
		}

		if _, duplicate := values[parts[2]]; duplicate {
			return nil, errors.New("duplicate NUT variable")
		}

		values[parts[2]] = parts[3]
		progress.variables++
	}

	return nil, errors.New("NUT variable list too large")
}

func (client *Client) listMarker(parts []string, marker string) bool {
	return len(parts) == 4 && parts[0] == marker && parts[1] == "LIST" && parts[2] == "VAR" && parts[3] == client.settings.UPS
}

func readingFrom(values map[string]string) (Reading, error) {
	charge := values["battery.charge"]
	battery, err := strconv.ParseFloat(charge, 64)
	if err != nil || !decimal.MatchString(charge) || !validBattery(battery) || len(strings.Fields(values["ups.status"])) == 0 {
		return Reading{}, errors.New("missing or invalid required UPS readings")
	}

	return Reading{At: time.Now(), Status: values["ups.status"], Battery: battery, Values: values}, nil
}

func (client *Client) write(command string) error {
	if _, err := fmt.Fprintln(client.connection, command); err != nil {
		return transportError("write", err, client.settings.ReadTimeout)
	}

	return nil
}

func (client *Client) readLine() ([]string, error) {
	line, err := client.reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, fmt.Errorf("NUT response line exceeds %d bytes", maxLineBytes)
	}

	if err != nil {
		return nil, transportError("read", err, client.settings.ReadTimeout)
	}

	parts, err := parseLine(strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r"))
	if err != nil {
		return nil, err
	}

	if len(parts) > 0 && parts[0] == "ERR" {
		return nil, serverError(parts)
	}

	return parts, nil
}

// Only known codes are reported so server text never reaches the logs.
func serverError(parts []string) error {
	if len(parts) > 1 {
		switch parts[1] {
		case "DATA-STALE", "DRIVER-NOT-CONNECTED", "UNKNOWN-UPS", "ACCESS-DENIED", "VAR-NOT-SUPPORTED":
			return fmt.Errorf("NUT %s", parts[1])
		}
	}

	return errors.New("NUT server rejected request")
}
