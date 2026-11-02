package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// CredentialNotice es el correo de la plataforma. No lleva secretos.
type CredentialNotice struct {
	Kicker      string
	Greeting    string
	Lead        string
	Notes       []string
	Rows        [][2]string
	ActionURL   string
	ActionLabel string
}

func (d *Dispatcher) SendCredentialConfirmation(to, subject string, note CredentialNotice) error {
	if d == nil || d.smtp == nil || !d.smtp.Enabled() {
		return fmt.Errorf("smtp sin configurar")
	}
	to = strings.TrimSpace(strings.ToLower(to))
	if to == "" || strings.TrimSpace(note.ActionURL) == "" {
		return fmt.Errorf("sin destinatario o enlace")
	}
	htmlBody, err := note.html()
	if err != nil {
		return err
	}
	return sendPlatformMessage(d.smtp, to, subject, note.plain(), htmlBody)
}

// SendPlatformLetter manda un aviso con la carta de GeldFlus. Sin SMTP, lo deja en el log.
func (d *Dispatcher) SendPlatformLetter(to, subject string, note CredentialNotice) error {
	if d == nil || d.smtp == nil || !d.smtp.Enabled() {
		log.Printf("[FlowPay email mock -> %s] %s | %s", to, subject, strings.ReplaceAll(note.plain(), "\n", " "))
		return nil
	}
	to = strings.TrimSpace(strings.ToLower(to))
	if to == "" {
		log.Printf("[FlowPay email] sin destinatario; no se envia")
		return nil
	}
	htmlBody, err := note.html()
	if err != nil {
		return err
	}
	return sendPlatformMessage(d.smtp, to, subject, note.plain(), htmlBody)
}

func (n CredentialNotice) plain() string {
	lines := []string{"GeldFlus", ""}
	if n.Greeting != "" {
		lines = append(lines, n.Greeting, "")
	}
	if n.Lead != "" {
		lines = append(lines, n.Lead)
	}
	if len(n.Rows) > 0 {
		lines = append(lines, "")
		for _, row := range n.Rows {
			lines = append(lines, row[0]+": "+row[1])
		}
	}
	if len(n.Notes) > 0 {
		lines = append(lines, "")
		for _, note := range n.Notes {
			lines = append(lines, "- "+note)
		}
	}
	if n.ActionURL != "" {
		label := strings.TrimSpace(n.ActionLabel)
		if label == "" {
			label = "Confirmar cambio"
		}
		lines = append(lines, "", label+": "+n.ActionURL)
	}
	lines = append(lines,
		"",
		"Saludos cordiales,",
		"Equipo GeldFlus",
		"Plataforma de cobranza",
		"contacto@geldflus.com",
		"www.geldflus.com",
	)
	return strings.Join(lines, "\r\n")
}

func (n CredentialNotice) html() (string, error) {
	var buf bytes.Buffer
	err := credentialLetterTmpl.Execute(&buf, n)
	return buf.String(), err
}

func sendPlatformMessage(cfg *SMTPConfig, to, subject, plain, htmlBody string) error {
	host := strings.TrimSpace(cfg.Host)
	port := strings.TrimSpace(cfg.Port)
	if port == "" {
		port = "587"
	}
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 12 * time.Second}
	var conn net.Conn
	var err error
	if port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("conectar %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("cliente smtp: %w", err)
	}
	defer client.Close()
	if port != "465" {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, host)); err != nil {
		return fmt.Errorf("auth smtp: %w", err)
	}
	from := strings.TrimSpace(cfg.From)
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(platformMessage(cfg, to, subject, plain, htmlBody)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func platformMessage(cfg *SMTPConfig, to, subject, plain, htmlBody string) []byte {
	subject = mime.QEncoding.Encode("utf-8", strings.NewReplacer("\r", "", "\n", "").Replace(subject))
	from := strings.TrimSpace(cfg.From)
	fromHeader := (&mail.Address{Name: "GeldFlus", Address: from}).String()
	const boundary = "geldflus-alternative"
	var buf bytes.Buffer
	buf.WriteString("From: " + fromHeader + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(`Content-Type: multipart/alternative; boundary="` + boundary + `"` + "\r\n")
	buf.WriteString("\r\n")
	writeBase64Part(&buf, boundary, "text/plain; charset=UTF-8", []byte(plain))
	writeBase64Part(&buf, boundary, "text/html; charset=UTF-8", []byte(htmlBody))
	buf.WriteString("--" + boundary + "--\r\n")
	return buf.Bytes()
}

func writeBase64Part(buf *bytes.Buffer, boundary, contentType string, body []byte) {
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: " + contentType + "\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	encoded := base64.StdEncoding.EncodeToString(body)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		buf.WriteString(encoded[i:end] + "\r\n")
	}
	buf.WriteString("\r\n")
}

var credentialLetterTmpl = template.Must(template.New("credential").Funcs(template.FuncMap{
	"add": func(a, b int) int { return a + b },
}).Parse(`<!DOCTYPE html>
<html lang="es">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#efeae3;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#efeae3;">
<tr><td align="center" style="padding:28px 12px;">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="width:600px;max-width:600px;background:#ffffff;border-radius:18px;overflow:hidden;">
<tr><td bgcolor="#0f6e6b" style="background:#0f6e6b;padding:26px 32px;">
<p style="margin:0;font-family:Georgia,'Times New Roman',serif;font-size:22px;letter-spacing:0.04em;color:#ffffff;">GeldFlus</p>
</td></tr>
<tr><td style="padding:34px 36px 8px;font-family:Arial,Helvetica,sans-serif;color:#1c1917;">
<p style="margin:0 0 8px;font-size:11px;letter-spacing:0.16em;text-transform:uppercase;color:#0f6e6b;">{{if .Kicker}}{{.Kicker}}{{else}}Credenciales de pago{{end}}</p>
{{if .Greeting}}<p style="margin:0;font-family:Georgia,'Times New Roman',serif;font-size:28px;line-height:1.25;color:#1c1917;">{{.Greeting}}</p>{{end}}
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:16px 0 0;"><tr><td style="width:42px;height:3px;background:#0f6e6b;font-size:0;line-height:0;">&nbsp;</td></tr></table>
{{if .Lead}}<p style="margin:22px 0 0;font-size:16px;line-height:1.7;color:#292524;">{{.Lead}}</p>{{end}}
{{if .ActionURL}}
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:24px 0 0;">
<tr><td bgcolor="#0f6e6b" style="background:#0f6e6b;border-radius:12px;">
<a href="{{.ActionURL}}" style="display:inline-block;padding:14px 22px;font-family:Arial,Helvetica,sans-serif;font-size:15px;font-weight:700;color:#ffffff;text-decoration:none;">{{if .ActionLabel}}{{.ActionLabel}}{{else}}Confirmar cambio{{end}}</a>
</td></tr>
</table>
<p style="margin:12px 0 0;font-size:12px;line-height:1.5;color:#8a837b;">Si el botón no abre, copie este enlace:<br>{{.ActionURL}}</p>
{{end}}
{{if .Notes}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:18px 0 0;background:#f7f4ef;border-radius:14px;">
<tr><td style="padding:6px 18px 8px;">
<p style="margin:12px 0 4px;font-size:11px;letter-spacing:0.14em;text-transform:uppercase;color:#6b645c;">Tenga en cuenta</p>
{{range $i, $line := .Notes}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 8px;">
<tr>
<td width="28" valign="top" style="width:28px;padding-top:2px;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td width="22" height="22" align="center" valign="middle" bgcolor="#0f6e6b" style="width:22px;height:22px;background:#0f6e6b;border-radius:11px;font-size:12px;font-weight:700;color:#ffffff;">{{add $i 1}}</td>
</tr></table>
</td>
<td valign="middle" style="padding:0 0 0 10px;font-size:14px;line-height:1.55;color:#3f3a36;">{{$line}}</td>
</tr>
</table>
{{end}}
</td></tr>
</table>
{{end}}
{{if .Rows}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:20px 0 0;border:1px solid #e7e1d8;border-radius:14px;">
{{range .Rows}}<tr>
<td style="padding:12px 16px;border-bottom:1px solid #efeae3;">
<p style="margin:0;font-size:11px;letter-spacing:0.12em;text-transform:uppercase;color:#6b645c;">{{index . 0}}</p>
<p style="margin:4px 0 0;font-size:15px;line-height:1.4;font-weight:700;color:#1c1917;">{{index . 1}}</p>
</td>
</tr>{{end}}
</table>
{{end}}
</td></tr>
<tr><td style="padding:28px 36px 32px;font-family:Georgia,'Times New Roman',serif;">
<p style="margin:0;border-top:1px solid #e7e1d8;padding-top:22px;font-size:16px;font-style:italic;color:#1c1917;">Saludos cordiales,</p>
<p style="margin:12px 0 0;font-size:18px;color:#1c1917;">Equipo GeldFlus</p>
<p style="margin:8px 0 0;font-family:Arial,Helvetica,sans-serif;font-size:13px;line-height:1.6;color:#8a837b;">
Plataforma de cobranza
<span style="color:#d6d0c8;">&nbsp;&middot;&nbsp;</span>
<a href="mailto:contacto@geldflus.com" style="color:#8a837b;text-decoration:none;">contacto@geldflus.com</a>
<span style="color:#d6d0c8;">&nbsp;&middot;&nbsp;</span>
<a href="https://www.geldflus.com" style="color:#8a837b;text-decoration:none;">www.geldflus.com</a>
</p>
</td></tr>
<tr><td style="background:#0f6e6b;padding:14px 32px;font-family:Arial,Helvetica,sans-serif;font-size:12px;line-height:1.5;color:#e7f3f2;">
GeldFlus · Mensaje confidencial. Si usted no esperaba este correo, puede ignorarlo.
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`))
