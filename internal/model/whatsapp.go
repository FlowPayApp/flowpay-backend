package model

import "time"

// WhatsAppNumber es el remitente/recurso Twilio asociado a una empresa.
type WhatsAppNumber struct {
	ID          int64     `json:"id"`
	CompanyID   int64     `json:"company_id"`
	PhoneNumber string    `json:"phone_number"`
	TwilioSID   string    `json:"twilio_sid"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// Message es un mensaje de chat WhatsApp persistido por empresa.
type Message struct {
	ID         int64     `json:"id"`
	CompanyID  int64     `json:"company_id"`
	ChargeID   *int64    `json:"charge_id,omitempty"`
	FromNumber string    `json:"from_number"`
	ToNumber   string    `json:"to_number"`
	Content    string    `json:"content"`
	Direction  string    `json:"direction"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}
