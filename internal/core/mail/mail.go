// Package mail is a tiny mailer with a stdlib net/smtp driver and a log driver
// (the safe default for dev). No runtime dependency beyond the standard library.
//
// It is OPTIONAL and self-configuring: a feature builds one on demand with
// mail.FromEnv(deps.Logger) — there is no central config field and no bootstrap
// wiring. Driver and credentials come from MAIL_* env vars.
package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// Message is an email to send. Provide Text, HTML, or both (both => a
// multipart/alternative message). From defaults to the mailer's configured From.
type Message struct {
	From    string
	To      []string
	Subject string
	Text    string
	HTML    string
}

func (m Message) validate() error {
	if len(m.To) == 0 {
		return errors.New("mail: message has no recipients")
	}
	if m.Text == "" && m.HTML == "" {
		return errors.New("mail: message has no Text or HTML body")
	}
	return nil
}

// Mailer sends messages. Implementations must be safe for concurrent use.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// LogMailer logs messages instead of sending them — the default driver, ideal for
// development and tests.
type LogMailer struct{ log *slog.Logger }

// Send records the message at info level.
func (m LogMailer) Send(ctx context.Context, msg Message) error {
	if err := msg.validate(); err != nil {
		return err
	}
	m.log.InfoContext(ctx, "mail (log driver — not sent)",
		slog.String("to", strings.Join(msg.To, ", ")),
		slog.String("subject", msg.Subject))
	return nil
}

// SMTPMailer sends via net/smtp (STARTTLS when the server offers it).
type SMTPMailer struct {
	addr string // host:port
	auth smtp.Auth
	from string // default From
}

// Send builds an RFC 5322 message and hands it to smtp.SendMail. The context is
// accepted for interface symmetry; net/smtp does not honour deadlines.
func (m *SMTPMailer) Send(_ context.Context, msg Message) error {
	if err := msg.validate(); err != nil {
		return err
	}
	from := msg.From
	if from == "" {
		from = m.from
	}
	raw, err := build(msg, from)
	if err != nil {
		return err
	}
	if err := smtp.SendMail(m.addr, m.auth, from, msg.To, raw); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return nil
}

// FromEnv builds a Mailer from MAIL_* env vars:
//
//	MAIL_DRIVER          log (default) | smtp
//	MAIL_FROM            default From address
//	MAIL_SMTP_HOST/PORT  smtp server (PORT default 587)
//	MAIL_SMTP_USERNAME   enables PlainAuth when set
//	MAIL_SMTP_PASSWORD
func FromEnv(log *slog.Logger) (Mailer, error) {
	from := getenv("MAIL_FROM", "no-reply@example.com")
	switch driver := getenv("MAIL_DRIVER", "log"); driver {
	case "log":
		return LogMailer{log: log}, nil
	case "smtp":
		host := os.Getenv("MAIL_SMTP_HOST")
		if host == "" {
			return nil, errors.New("mail: MAIL_SMTP_HOST is required for the smtp driver")
		}
		var auth smtp.Auth
		if user := os.Getenv("MAIL_SMTP_USERNAME"); user != "" {
			auth = smtp.PlainAuth("", user, os.Getenv("MAIL_SMTP_PASSWORD"), host)
		}
		return &SMTPMailer{
			addr: net.JoinHostPort(host, getenv("MAIL_SMTP_PORT", "587")),
			auth: auth,
			from: from,
		}, nil
	default:
		return nil, fmt.Errorf("mail: unknown MAIL_DRIVER %q (want log|smtp)", driver)
	}
}

// build renders a message to RFC 5322 bytes (headers + MIME body).
func build(msg Message, from string) ([]byte, error) {
	if err := msg.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + strings.Join(msg.To, ", ") + "\r\n")
	buf.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n")
	buf.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")

	if msg.Text != "" && msg.HTML != "" {
		mw := multipart.NewWriter(&buf)
		buf.WriteString("Content-Type: multipart/alternative; boundary=" + mw.Boundary() + "\r\n\r\n")
		if err := writePart(mw, "text/plain; charset=utf-8", msg.Text); err != nil {
			return nil, err
		}
		if err := writePart(mw, "text/html; charset=utf-8", msg.HTML); err != nil {
			return nil, err
		}
		if err := mw.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	ct, body := "text/plain; charset=utf-8", msg.Text
	if msg.HTML != "" {
		ct, body = "text/html; charset=utf-8", msg.HTML
	}
	buf.WriteString("Content-Type: " + ct + "\r\n\r\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}

func writePart(mw *multipart.Writer, contentType, body string) error {
	pw, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {contentType}})
	if err != nil {
		return err
	}
	_, err = pw.Write([]byte(body))
	return err
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
