package model

import "time"

// CompanyMailbox es el remitente SMTP de una empresa. La contraseña no sale en JSON.
type CompanyMailbox struct {
	ID           int64     `json:"id"`
	CompanyID    int64     `json:"company_id"`
	FromName     string    `json:"from_name"`
	FromEmail    string    `json:"from_email"`
	SMTPHost     string    `json:"smtp_host"`
	SMTPPort     string    `json:"smtp_port"`
	SMTPUsername string    `json:"smtp_username"`
	SMTPPassword string    `json:"-"`
	PasswordSet  bool      `json:"password_set"`
	UpdatedAt    time.Time `json:"updated_at"`
}
