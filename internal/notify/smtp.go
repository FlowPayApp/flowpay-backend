package notify

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

// SMTPConfig para Gmail, Outlook, etc. (STARTTLS puerto 587).
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	FromName string
}

// Enabled hay credenciales suficientes para enviar.
func (c *SMTPConfig) Enabled() bool {
	return c != nil && strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.Username) != "" && strings.TrimSpace(c.Password) != "" && strings.TrimSpace(c.From) != ""
}

func (d *Dispatcher) sendEmail(to, subject, body string) {
	_ = d.sendEmailWith(nil, to, subject, body)
}

// sendEmailWith usa el buzón de la empresa si está completo; si no, el SMTP global.
func (d *Dispatcher) sendEmailWith(mailbox *SMTPConfig, to, subject, body string) error {
	to = strings.TrimSpace(to)
	subject = strings.NewReplacer("\r", "", "\n", "").Replace(strings.TrimSpace(subject))
	if to == "" {
		log.Printf("[FlowPay email] sin destinatario; no se envia")
		return nil
	}
	cfg := mailbox
	if cfg == nil || !cfg.Enabled() {
		cfg = d.smtp
	}
	if cfg == nil || !cfg.Enabled() {
		log.Printf("[FlowPay email mock -> %s] %s | %s", to, subject, strings.ReplaceAll(body, "\n", " "))
		return nil
	}
	port := strings.TrimSpace(cfg.Port)
	if port == "" {
		port = "587"
	}
	fromHeader := strings.TrimSpace(cfg.From)
	if name := strings.TrimSpace(cfg.FromName); name != "" {
		fromHeader = name + " <" + fromHeader + ">"
	}
	addr := fmt.Sprintf("%s:%s", strings.TrimSpace(cfg.Host), port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		fromHeader, to, subject, body,
	)
	err := smtp.SendMail(addr, auth, strings.TrimSpace(cfg.From), []string{to}, []byte(msg))
	if err != nil {
		log.Printf("[FlowPay email] error enviando a %s desde %s: %v", to, cfg.From, err)
		return fmt.Errorf("no se pudo enviar el correo desde %s", strings.TrimSpace(cfg.From))
	}
	log.Printf("[FlowPay email] enviado a %s desde %s", to, cfg.From)
	return nil
}
