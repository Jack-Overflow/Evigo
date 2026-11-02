package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

type fakeSMTPServer struct {
	listener       net.Listener
	certificate    tls.Certificate
	rootCAs        *x509.CertPool
	implicitTLS    bool
	offerStartTLS  bool
	rejectedRcpt   string
	silent         bool
	mutex          sync.Mutex
	messages       []string
	recipients     []string
	credentials    string
	securedOnLogin bool
}

func startFakeSMTPServer(t *testing.T, configure func(*fakeSMTPServer)) *fakeSMTPServer {
	t.Helper()
	certificateServer := httptest.NewUnstartedServer(nil)
	certificateServer.StartTLS()
	t.Cleanup(certificateServer.Close)
	rootCAs := x509.NewCertPool()
	rootCAs.AddCert(certificateServer.Certificate())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { listener.Close() })
	server := &fakeSMTPServer{listener: listener, certificate: certificateServer.TLS.Certificates[0], rootCAs: rootCAs}
	configure(server)
	go server.accept()

	return server
}

func (server *fakeSMTPServer) tlsConfig() *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{server.certificate}}
}

func (server *fakeSMTPServer) accept() {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}

		go server.serve(connection)
	}
}

func (server *fakeSMTPServer) serve(connection net.Conn) {
	defer connection.Close()
	if server.silent {
		io.Copy(io.Discard, connection)

		return
	}

	secured := server.implicitTLS
	if server.implicitTLS {
		connection = tls.Server(connection, server.tlsConfig())
	}

	reader := textproto.NewReader(bufio.NewReader(connection))
	reply := func(text string) { fmt.Fprintf(connection, "%s\r\n", text) }
	reply("220 fake ready")
	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}

		command := strings.ToUpper(strings.Fields(line + " x")[0])
		switch command {
		case "EHLO":
			reply("250-fake")
			if server.offerStartTLS && !secured {
				reply("250-STARTTLS")
			}
			reply("250 AUTH PLAIN")
		case "STARTTLS":
			reply("220 go ahead")
			connection = tls.Server(connection, server.tlsConfig())
			reader = textproto.NewReader(bufio.NewReader(connection))
			secured = true
		case "AUTH":
			fields := strings.Fields(line)
			decoded, _ := base64.StdEncoding.DecodeString(fields[len(fields)-1])
			server.mutex.Lock()
			server.credentials = string(decoded)
			server.securedOnLogin = secured
			server.mutex.Unlock()
			reply("235 ok")
		case "MAIL":
			reply("250 ok")
		case "RCPT":
			if server.rejectedRcpt != "" && strings.Contains(line, server.rejectedRcpt) {
				reply("550 no such user " + server.rejectedRcpt)

				continue
			}

			server.mutex.Lock()
			server.recipients = append(server.recipients, line)
			server.mutex.Unlock()
			reply("250 ok")
		case "DATA":
			reply("354 go ahead")
			lines, err := reader.ReadDotLines()
			if err != nil {
				return
			}

			server.mutex.Lock()
			server.messages = append(server.messages, strings.Join(lines, "\r\n"))
			server.mutex.Unlock()
			reply("250 ok")
		case "QUIT":
			reply("221 bye")

			return
		default:
			reply("500 unknown")
		}
	}
}

func (server *fakeSMTPServer) newSender(security Security, username, password string) *Sender {
	host, portText, _ := net.SplitHostPort(server.listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	sender := New(Settings{
		Host:     host,
		Port:     port,
		Security: security,
		Username: username,
		Password: password,
		From:     "evigo@example.lan",
		To:       []string{"first@example.lan", "second@example.lan"},
	})
	sender.rootCAs = server.rootCAs

	return sender
}

func (server *fakeSMTPServer) savedCredentials() string {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	return server.credentials
}

func (server *fakeSMTPServer) savedMessages() []string {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	return slices.Clone(server.messages)
}

func (server *fakeSMTPServer) savedRecipients() []string {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	return slices.Clone(server.recipients)
}

func (server *fakeSMTPServer) loginWasSecured() bool {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	return server.securedOnLogin
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	return ctx
}

var happenedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func TestGivenStartTLSServerWhenAlertIsSentThenItAuthenticatesOverTLSAndDeliversTheMessage(t *testing.T) {
	// Given a server offering STARTTLS and a sender with credentials.
	server := startFakeSMTPServer(t, func(server *fakeSMTPServer) { server.offerStartTLS = true })
	sender := server.newSender(StartTLS, "monitor", "smtp-password")

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.DeviceGaveUp(happenedAt, "Home server", 3))
	// Then the login happens over TLS and every recipient gets the message.
	if err != nil {
		t.Fatal(err)
	}
	if !server.loginWasSecured() || !strings.Contains(server.savedCredentials(), "monitor") {
		t.Fatalf("secured %t, credentials present %t", server.loginWasSecured(), server.savedCredentials() != "")
	}
	if len(server.savedRecipients()) != 2 || len(server.savedMessages()) != 1 {
		t.Fatalf("recipients %d, messages %d", len(server.savedRecipients()), len(server.savedMessages()))
	}

	message, err := mail.ReadMessage(strings.NewReader(server.savedMessages()[0] + "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if subject := message.Header.Get("Subject"); subject != "[Evigo] Home server did not come back" {
		t.Fatalf("subject %q", subject)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Home server did not answer ping after 3 wake attempts.") || !strings.Contains(string(body), "2026-01-02 03:04:05 UTC") {
		t.Fatalf("body %q", body)
	}
}

func TestGivenServerWithoutStartTLSWhenStartTLSIsRequiredThenNothingIsSentInTheClear(t *testing.T) {
	// Given a server that does not offer STARTTLS.
	server := startFakeSMTPServer(t, func(*fakeSMTPServer) {})
	sender := server.newSender(StartTLS, "monitor", "smtp-password")

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.OnBattery(happenedAt, 90))

	// Then it fails rather than downgrading, and neither login nor message is sent.
	if err == nil {
		t.Fatal("sent without the required TLS")
	}
	if server.savedCredentials() != "" || len(server.savedMessages()) != 0 {
		t.Fatal("sent data over an unencrypted connection")
	}
}

func TestGivenImplicitTLSServerWhenAlertIsSentThenItIsDelivered(t *testing.T) {
	// Given a server that speaks TLS from the first byte.
	server := startFakeSMTPServer(t, func(server *fakeSMTPServer) { server.implicitTLS = true })
	sender := server.newSender(ImplicitTLS, "monitor", "smtp-password")

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.OnBattery(happenedAt, 90))
	// Then the message is delivered and the login was encrypted.
	if err != nil {
		t.Fatal(err)
	}
	if !server.loginWasSecured() || len(server.savedMessages()) != 1 {
		t.Fatalf("secured %t, messages %d", server.loginWasSecured(), len(server.savedMessages()))
	}
}

func TestGivenUntrustedCertificateWhenAlertIsSentThenNothingIsDelivered(t *testing.T) {
	// Given a server whose certificate the sender does not trust.
	server := startFakeSMTPServer(t, func(server *fakeSMTPServer) { server.offerStartTLS = true })
	sender := server.newSender(StartTLS, "monitor", "smtp-password")
	sender.rootCAs = x509.NewCertPool()

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.OnBattery(happenedAt, 90))

	// Then it fails before any login or message is sent.
	if err == nil || server.savedCredentials() != "" || len(server.savedMessages()) != 0 {
		t.Fatalf("err %v, credentials present %t, messages %d", err, server.savedCredentials() != "", len(server.savedMessages()))
	}
}

func TestGivenRelayWithNoTLSWhenAlertIsSentThenItIsDeliveredWithoutALogin(t *testing.T) {
	// Given a trusted relay without TLS or credentials.
	server := startFakeSMTPServer(t, func(*fakeSMTPServer) {})
	sender := server.newSender(NoTLS, "", "")

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.OnBattery(happenedAt, 90))
	// Then the message is delivered without attempting a login.
	if err != nil {
		t.Fatal(err)
	}
	if server.savedCredentials() != "" || len(server.savedMessages()) != 1 {
		t.Fatalf("credentials present %t, messages %d", server.savedCredentials() != "", len(server.savedMessages()))
	}
}

func TestGivenRejectedRecipientWhenAlertIsSentThenTheErrorKeepsOnlyTheReplyCode(t *testing.T) {
	// Given a server whose rejection repeats an address.
	server := startFakeSMTPServer(t, func(server *fakeSMTPServer) { server.rejectedRcpt = "second@example.lan" })
	sender := server.newSender(NoTLS, "", "")

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.OnBattery(happenedAt, 90))

	// Then the error has the code but none of the server's text.
	if err == nil || !strings.Contains(err.Error(), "550") || strings.Contains(err.Error(), "example.lan") {
		t.Fatalf("error %v", err)
	}
}

func TestGivenServerThatNeverRepliesWhenTheDeadlinePassesThenSendReturns(t *testing.T) {
	// Given a server that accepts connections but never sends a greeting.
	server := startFakeSMTPServer(t, func(server *fakeSMTPServer) { server.silent = true })
	sender := server.newSender(NoTLS, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// When an alert is sent.
	finished := make(chan error, 1)
	go func() { finished <- sender.Send(ctx, alerts.OnBattery(happenedAt, 90)) }()

	// Then it returns an error once the deadline passes rather than hanging.
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("reported success from a silent server")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send ignored its deadline")
	}
}

func TestGivenDeviceNameWithLineBreaksOrAccentsWhenAlertIsSentThenTheSubjectStaysOneHeader(t *testing.T) {
	// Given a device name containing a line break and a non-ASCII character.
	server := startFakeSMTPServer(t, func(*fakeSMTPServer) {})
	sender := server.newSender(NoTLS, "", "")

	// When an alert is sent.
	err := sender.Send(testContext(t), alerts.DeviceRecovered(happenedAt, "Sérver\r\nBcc: attacker@example.lan", 1))
	// Then no header is injected and the name decodes back intact.
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(strings.NewReader(server.savedMessages()[0] + "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if message.Header.Get("Bcc") != "" {
		t.Fatal("header injected through the device name")
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil || !strings.Contains(subject, "Sérver") {
		t.Fatalf("subject %q, error %v", subject, err)
	}
}
