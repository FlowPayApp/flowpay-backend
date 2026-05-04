package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/models"
)

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
