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
}

func (c *SMTPConfig) enabled() bool {
	return c != nil && c.Host != "" && c.Username != "" && c.Password != "" && c.From != ""
}

func (d *Dispatcher) sendEmail(to, subject, body string) {
	if to == "" {
		log.Printf("[FlowPay email] sin destinatario; no se envia")
		return
	}
	if d.smtp == nil || !d.smtp.enabled() {
		log.Printf("[FlowPay email mock -> %s] %s | %s", to, subject, strings.ReplaceAll(body, "\n", " "))
		return
	}
	port := d.smtp.Port
	if port == "" {
		port = "587"
	}
	addr := fmt.Sprintf("%s:%s", d.smtp.Host, port)
	auth := smtp.PlainAuth("", d.smtp.Username, d.smtp.Password, d.smtp.Host)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		d.smtp.From, to, subject, body,
	)
	err := smtp.SendMail(addr, auth, d.smtp.From, []string{to}, []byte(msg))
	if err != nil {
		log.Printf("[FlowPay email] error enviando a %s: %v", to, err)
		return
	}
	log.Printf("[FlowPay email] enviado a %s", to)
}
