package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/models"
)

var ErrNoActiveWhatsAppNumber = errors.New("empresa sin número WhatsApp activo")

// GetActiveWhatsAppNumber devuelve el primer número activo de la empresa (orden por id).
func (r *Repository) GetActiveWhatsAppNumber(ctx context.Context, companyID int64) (*models.WhatsAppNumber, error) {
	q := `
SELECT id, company_id, phone_number, twilio_sid, status, created_at
FROM whatsapp_numbers
WHERE company_id = ? AND status = 'active'
ORDER BY id ASC
LIMIT 1
`
	var w models.WhatsAppNumber
	err := r.db.QueryRowContext(ctx, q, companyID).Scan(
		&w.ID, &w.CompanyID, &w.PhoneNumber, &w.TwilioSID, &w.Status, &w.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoActiveWhatsAppNumber
		}
		return nil, err
	}
	return &w, nil
}

// FindWhatsAppNumberByTo busca el tenant por el número receptor (normalizado whatsapp:+...).
func (r *Repository) FindWhatsAppNumberByTo(ctx context.Context, toNormalized string) (*models.WhatsAppNumber, error) {
	toNormalized = strings.TrimSpace(toNormalized)
	if toNormalized == "" {
		return nil, sql.ErrNoRows
	}
	q := `
SELECT id, company_id, phone_number, twilio_sid, status, created_at
FROM whatsapp_numbers
WHERE LOWER(phone_number) = LOWER(?) AND status = 'active'
LIMIT 1
`
	var w models.WhatsAppNumber
	err := r.db.QueryRowContext(ctx, q, toNormalized).Scan(
		&w.ID, &w.CompanyID, &w.PhoneNumber, &w.TwilioSID, &w.Status, &w.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// InsertMessage guarda un mensaje de WhatsApp.
func (r *Repository) InsertMessage(ctx context.Context, m *models.Message) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO messages (company_id, from_number, to_number, content, direction, status) VALUES (?, ?, ?, ?, ?, ?)`,
		m.CompanyID, m.FromNumber, m.ToNumber, m.Content, m.Direction, m.Status,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListMessagesByCompany lista mensajes del tenant más recientes primero.
// Si phoneFilter viene con valor, filtra por conversación del número (from/to).
func (r *Repository) ListMessagesByCompany(ctx context.Context, companyID int64, limit, offset int, phoneFilter string) ([]models.Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	phoneFilter = strings.TrimSpace(phoneFilter)
	if phoneFilter == "" {
		q := `
SELECT id, company_id, from_number, to_number, content, direction, status, created_at
FROM messages
WHERE company_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?
`
		rows, err := r.db.QueryContext(ctx, q, companyID, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []models.Message
		for rows.Next() {
			var m models.Message
			if err := rows.Scan(&m.ID, &m.CompanyID, &m.FromNumber, &m.ToNumber, &m.Content, &m.Direction, &m.Status, &m.CreatedAt); err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, rows.Err()
	}
	q := `
SELECT id, company_id, from_number, to_number, content, direction, status, created_at
FROM messages
WHERE company_id = ?
  AND (
    REPLACE(LOWER(from_number), 'whatsapp:', '') = REPLACE(LOWER(?), 'whatsapp:', '')
    OR REPLACE(LOWER(to_number), 'whatsapp:', '') = REPLACE(LOWER(?), 'whatsapp:', '')
  )
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?
`
	rows, err := r.db.QueryContext(ctx, q, companyID, phoneFilter, phoneFilter, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Message
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(&m.ID, &m.CompanyID, &m.FromNumber, &m.ToNumber, &m.Content, &m.Direction, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
