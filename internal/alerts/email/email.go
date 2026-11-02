package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

type Security string

const (
	StartTLS    Security = "starttls"
	ImplicitTLS Security = "tls"
	NoTLS       Security = "none"
)

type Settings struct {
	Host     string
	Port     int
	Security Security
	Username string
	Password string
	From     string
	To       []string
}

type Sender struct {
	settings Settings
	rootCAs  *x509.CertPool
}

func New(settings Settings) *Sender {
	return &Sender{settings: settings}
}

func (sender *Sender) Send(ctx context.Context, alert alerts.Alert) error {
	connection, err := sender.connect(ctx)
	if err != nil {
		return failed("connection", err)
	}

	defer connection.Close()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return failed("deadline", err)
	}
	stopWatchingContext := context.AfterFunc(ctx, func() { connection.Close() })
	defer stopWatchingContext()

	client, err := smtp.NewClient(connection, sender.settings.Host)
	if err != nil {
		return failed("greeting", err)
	}

	defer client.Close()
	if err = sender.secureAndAuthenticate(client); err != nil {
		return err
	}

	if err = sender.deliver(client, alert); err != nil {
		return err
	}

	// The server has accepted the message, so a failure here must not cause a duplicate.
	_ = client.Quit()

	return nil
}

func (sender *Sender) connect(ctx context.Context) (net.Conn, error) {
	address := net.JoinHostPort(sender.settings.Host, strconv.Itoa(sender.settings.Port))
	if sender.settings.Security == ImplicitTLS {
		dialer := tls.Dialer{Config: sender.tlsConfig()}

		return dialer.DialContext(ctx, "tcp", address)
	}

	var dialer net.Dialer

	return dialer.DialContext(ctx, "tcp", address)
}

func (sender *Sender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: sender.settings.Host, RootCAs: sender.rootCAs, MinVersion: tls.VersionTLS12}
}

func (sender *Sender) secureAndAuthenticate(client *smtp.Client) error {
	if sender.settings.Security == StartTLS {
		if offered, _ := client.Extension("STARTTLS"); !offered {
			return errors.New("email: SMTP server does not offer STARTTLS")
		}

		if err := client.StartTLS(sender.tlsConfig()); err != nil {
			return failed("STARTTLS", err)
		}
	}

	if sender.settings.Username == "" {
		return nil
	}

	credentials := smtp.PlainAuth("", sender.settings.Username, sender.settings.Password, sender.settings.Host)
	if err := client.Auth(credentials); err != nil {
		return failed("authentication", err)
	}

	return nil
}

func (sender *Sender) deliver(client *smtp.Client, alert alerts.Alert) error {
	if err := client.Mail(sender.settings.From); err != nil {
		return failed("sender address", err)
	}

	for _, recipient := range sender.settings.To {
		if err := client.Rcpt(recipient); err != nil {
			return failed("recipient address", err)
		}
	}

	message, err := sender.buildMessage(alert)
	if err != nil {
		return failed("message", err)
	}

	writer, err := client.Data()
	if err != nil {
		return failed("message", err)
	}

	if _, err = writer.Write(message); err != nil {
		return failed("message", err)
	}

	if err = writer.Close(); err != nil {
		return failed("message", err)
	}

	return nil
}

func (sender *Sender) buildMessage(alert alerts.Alert) ([]byte, error) {
	var message bytes.Buffer
	fromDomain := sender.settings.From[strings.LastIndex(sender.settings.From, "@")+1:]
	fmt.Fprintf(&message, "From: %s\r\n", sender.settings.From)
	fmt.Fprintf(&message, "To: %s\r\n", strings.Join(sender.settings.To, ", "))
	fmt.Fprintf(&message, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", "[Evigo] "+alert.Title))
	fmt.Fprintf(&message, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&message, "Message-ID: <%s@%s>\r\n", rand.Text(), fromDomain)
	message.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")

	body := quotedprintable.NewWriter(&message)
	if _, err := fmt.Fprintf(body, "%s\n\nHappened at %s.\n", alert.Body, alert.At.Format("2006-01-02 15:04:05 MST")); err != nil {
		return nil, err
	}

	if err := body.Close(); err != nil {
		return nil, err
	}

	return message.Bytes(), nil
}

// SMTP replies can repeat addresses and server text, so only the reply code is kept.
func failed(step string, err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return fmt.Errorf("email: SMTP %s failed with code %d", step, reply.Code)
	}

	return fmt.Errorf("email: SMTP %s failed", step)
}
