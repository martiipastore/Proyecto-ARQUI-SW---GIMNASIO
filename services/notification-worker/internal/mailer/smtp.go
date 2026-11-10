package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Email struct{ To, Subject, Body, MessageID string }

type SMTP struct {
	Host, Port, From, Username, Password string
	AllowPlaintext                       bool
}

func (s SMTP) Send(ctx context.Context, m Email) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil || strings.ContainsAny(s.From, "\r\n") {
		return fmt.Errorf("remitente inválido")
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil || strings.ContainsAny(m.To, "\r\n") {
		return fmt.Errorf("destinatario inválido")
	}
	if strings.ContainsAny(m.Subject+m.MessageID, "\r\n") {
		return fmt.Errorf("cabecera inválida")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return fmt.Errorf("SMTP no disponible")
	}
	defer func() { _ = conn.Close() }()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	encrypted := false
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
		encrypted = true
	} else if !s.AllowPlaintext || (s.Host != "mailpit" && s.Host != "localhost" && s.Host != "127.0.0.1" && s.Host != "::1") {
		return fmt.Errorf("SMTP requiere TLS")
	}
	if s.Username != "" {
		if !encrypted {
			return fmt.Errorf("autenticación SMTP requiere TLS")
		}
		if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err = client.Mail(from.Address); err != nil {
		return err
	}
	if err = client.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	body := "From: " + from.String() + "\r\nTo: " + to.String() + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\nMessage-ID: <" + m.MessageID + "@gym.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + strings.ReplaceAll(m.Body, "\n", "\r\n")
	if _, err = w.Write([]byte(body)); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
