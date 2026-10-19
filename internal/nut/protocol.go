package nut

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"
)

var errUnterminated = errors.New("unterminated NUT response")

func parseLine(line string) ([]string, error) {
	var parts []string
	for position := 0; position < len(line); {
		switch line[position] {
		case ' ', '\t':
			position++
		case '#':
			return parts, nil
		case '=':
			parts = append(parts, "=")
			position++
		default:
			token, next, err := parseToken(line, position)
			if err != nil {
				return nil, err
			}

			parts = append(parts, token)
			position = next
		}
	}

	return parts, nil
}

func parseToken(line string, position int) (string, int, error) {
	quoted := line[position] == '"'
	if quoted {
		position++
	}

	var token strings.Builder
	for position < len(line) {
		character := line[position]
		switch {
		case character == '\\':
			if position+1 == len(line) {
				return "", 0, errors.New("invalid NUT escape")
			}

			token.WriteByte(line[position+1])
			position += 2

			continue
		case quoted && character == '"':
			return token.String(), position + 1, nil
		case !quoted && strings.IndexByte(" \t#=", character) >= 0:
			return token.String(), position, nil
		case character < 32 || character > 126:
			return "", 0, errors.New("invalid NUT response character")
		}

		token.WriteByte(character)
		position++
	}

	if quoted {
		return "", 0, errUnterminated
	}

	return token.String(), position, nil
}

func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// The underlying error text can contain hostnames or addresses.
func transportError(operation string, err error, timeout time.Duration) error {
	return fmt.Errorf("NUT %s %s", operation, transportReason(err, timeout))
}

func transportReason(err error, timeout time.Duration) string {
	var networkError net.Error
	var dnsError *net.DNSError
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout():
		return "timed out after " + timeout.String()
	case errors.As(err, &dnsError):
		return "DNS resolution failed"
	case errors.Is(err, io.EOF):
		return "server closed the connection (EOF)"
	case errors.Is(err, net.ErrClosed):
		return "connection closed locally"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection reset by peer"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, syscall.EPIPE):
		return "broken pipe"
	}

	return "I/O failed"
}
