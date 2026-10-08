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
	ID         int64          `json:"id"`
	CompanyID  int64          `json:"company_id"`
	ChargeID   *int64         `json:"charge_id,omitempty"`
	FromNumber string         `json:"from_number"`
	ToNumber   string         `json:"to_number"`
	Content    string         `json:"content"`
	Media      []MessageMedia `json:"media"`
	Direction  string         `json:"direction"`
	Status     string         `json:"status"`
	ReadAt     *time.Time     `json:"read_at,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// MessageMedia es un adjunto de WhatsApp (foto, audio, PDF...).
// Los recibidos viven en Twilio (URL con las credenciales de la cuenta); los enviados, en message_files.
// Ninguna de las dos referencias sale del servidor.
type MessageMedia struct {
	ContentType string `json:"content_type"`
	FileName    string `json:"file_name,omitempty"`
	URL         string `json:"-"`
	FileToken   string `json:"-"`
}

// MessageFile es un archivo que la empresa envió por WhatsApp.
type MessageFile struct {
	ContentType string
	FileName    string
	Data        []byte
}

// UnreadThread resume las respuestas sin leer de un cobro.
type UnreadThread struct {
	ChargeID   int64     `json:"charge_id"`
	ClientName string    `json:"client_name"`
	Unread     int       `json:"unread"`
	LastAt     time.Time `json:"last_at"`
	Preview    string    `json:"preview"`
	HasMedia   bool      `json:"has_media"`
}

// Inbox respuestas de clientes que nadie ha abierto.
type Inbox struct {
	Total   int            `json:"total"`
	Threads []UnreadThread `json:"threads"`
}
