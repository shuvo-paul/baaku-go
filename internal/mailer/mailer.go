// Package mailer sends the auth emails (password reset, email verification)
// over SMTP using only the standard library. Templates and wording live in
// this package, mirroring the reference app's Laravel notifications — not in
// templ views.
package mailer

import (
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/smtp"
	"time"
)

// Config is the SMTP settings loaded by internal/config (MAIL_* env vars,
// mirroring reference/config/mail.php).
type Config struct {
	Host     string // MAIL_HOST
	Port     string // MAIL_PORT
	Username string // MAIL_USERNAME
	Password string // MAIL_PASSWORD
	From     string // MAIL_FROM, bare address
}

// Send delivers one HTML email to a single recipient. Port 465 uses implicit
// TLS; any other port gets STARTTLS when the server advertises it.
func Send(cfg Config, to, subject, htmlBody string) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	conn, err := dial(addr, cfg)
	if err != nil {
		return fmt.Errorf("mailer: dial %s: %w", addr, err)
	}
	// ponytail: no deadline once the session is up — add a conn deadline
	// if a hung SMTP server ever stalls a caller.
	defer conn.Close()

	cl, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("mailer: smtp client: %w", err)
	}
	defer cl.Close()

	if ok, _ := cl.Extension("STARTTLS"); ok {
		if err := cl.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return fmt.Errorf("mailer: starttls: %w", err)
		}
	}
	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := cl.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err := cl.Mail(cfg.From); err != nil {
		return fmt.Errorf("mailer: mail from: %w", err)
	}
	if err := cl.Rcpt(to); err != nil {
		return fmt.Errorf("mailer: rcpt to: %w", err)
	}

	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("mailer: data: %w", err)
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		cfg.From, to, mime.QEncoding.Encode("utf-8", subject), htmlBody)
	if _, err := io.WriteString(w, msg); err != nil {
		return fmt.Errorf("mailer: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: finish body: %w", err)
	}
	return cl.Quit()
}

func dial(addr string, cfg Config) (net.Conn, error) {
	d := &net.Dialer{Timeout: 10 * time.Second}
	if cfg.Port == "465" {
		return tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: cfg.Host})
	}
	return d.Dial("tcp", addr)
}
