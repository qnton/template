package mail

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestBuildSinglePart(t *testing.T) {
	raw, err := build(Message{To: []string{"a@example.com"}, Subject: "Hi", Text: "hello"}, "from@example.com")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"From: from@example.com\r\n",
		"To: a@example.com\r\n",
		"Subject: Hi\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n\r\n",
		"hello",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q\n---\n%s", want, s)
		}
	}
}

func TestBuildMultipart(t *testing.T) {
	raw, err := build(Message{To: []string{"a@example.com", "b@example.com"}, Subject: "Multi", Text: "plain", HTML: "<b>rich</b>"}, "from@example.com")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"To: a@example.com, b@example.com\r\n",
		"multipart/alternative; boundary=",
		"text/plain; charset=utf-8",
		"text/html; charset=utf-8",
		"plain",
		"<b>rich</b>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q\n---\n%s", want, s)
		}
	}
}

func TestBuildValidation(t *testing.T) {
	if _, err := build(Message{Subject: "x", Text: "y"}, "f@x.com"); err == nil {
		t.Error("expected error for no recipients")
	}
	if _, err := build(Message{To: []string{"a@x.com"}, Subject: "x"}, "f@x.com"); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestLogMailerSends(t *testing.T) {
	var buf bytes.Buffer
	m := LogMailer{log: slog.New(slog.NewTextHandler(&buf, nil))}
	if err := m.Send(context.Background(), Message{To: []string{"a@example.com"}, Subject: "Welcome", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "Welcome") || !strings.Contains(out, "a@example.com") {
		t.Errorf("log driver did not record the message: %s", out)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "log")
	if m, err := FromEnv(slog.Default()); err != nil {
		t.Fatalf("log driver: %v", err)
	} else if _, ok := m.(LogMailer); !ok {
		t.Errorf("default driver = %T, want LogMailer", m)
	}

	t.Setenv("MAIL_DRIVER", "smtp")
	t.Setenv("MAIL_SMTP_HOST", "")
	if _, err := FromEnv(slog.Default()); err == nil {
		t.Error("smtp driver without MAIL_SMTP_HOST should error")
	}

	t.Setenv("MAIL_SMTP_HOST", "smtp.example.com")
	if m, err := FromEnv(slog.Default()); err != nil {
		t.Fatalf("smtp driver: %v", err)
	} else if _, ok := m.(*SMTPMailer); !ok {
		t.Errorf("smtp driver = %T, want *SMTPMailer", m)
	}

	t.Setenv("MAIL_DRIVER", "carrierpigeon")
	if _, err := FromEnv(slog.Default()); err == nil {
		t.Error("unknown driver should error")
	}
}
