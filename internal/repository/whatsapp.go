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
WHERE LOWER(phone_number) = LOWER($1) AND status = 'active'
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

// FirstActiveWhatsAppToForCompany primer número Twilio activo de la empresa (receptor en webhooks).
func (r *Repository) FirstActiveWhatsAppToForCompany(ctx context.Context, companyID int64) (string, error) {
	var phone string
	err := r.db.QueryRowContext(ctx, `
SELECT phone_number FROM whatsapp_numbers
WHERE company_id = $1 AND status = 'active'
ORDER BY id ASC LIMIT 1
`, companyID).Scan(&phone)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(phone), nil
}

// GetMessageByID mensaje por id y empresa (cualquier dirección).
func (r *Repository) GetMessageByID(ctx context.Context, companyID, msgID int64) (*models.Message, error) {
	q := `
SELECT id, company_id, charge_id, from_number, to_number, content, direction, status, created_at
FROM messages WHERE id = $1 AND company_id = $2
`
	var m models.Message
	var charge sql.NullInt64
	err := r.db.QueryRowContext(ctx, q, msgID, companyID).Scan(
		&m.ID, &m.CompanyID, &charge, &m.FromNumber, &m.ToNumber, &m.Content, &m.Direction, &m.Status, &m.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if charge.Valid {
		v := charge.Int64
		m.ChargeID = &v
	}
	return &m, nil
}

// FindOpenChargeIDForInboundWhatsApp elige un cobro pendiente del cliente cuyo teléfono coincide con from (Twilio).
// Prioriza el cobro creado más recientemente.
func (r *Repository) FindOpenChargeIDForInboundWhatsApp(ctx context.Context, companyID int64, fromNormalized string) (*int64, error) {
	q := `
SELECT ch.id, COALESCE(c.phone, '') AS phone
FROM charges ch
JOIN clients c ON c.id = ch.client_id
WHERE ch.company_id = $1
  AND ch.paid_at IS NULL
  AND c.phone IS NOT NULL AND TRIM(c.phone) <> ''
ORDER BY ch.created_at DESC, ch.id DESC
`
	rows, err := r.db.QueryContext(ctx, q, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var phone string
		if err := rows.Scan(&id, &phone); err != nil {
			return nil, err
		}
		if phonesLikelyMatch(fromNormalized, phone) {
			return &id, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ListInboundMessagesForCharge mensajes entrantes de WhatsApp asociados al cobro.
func (r *Repository) ListInboundMessagesForCharge(ctx context.Context, companyID, chargeID int64) ([]models.Message, error) {
	q := `
SELECT id, company_id, charge_id, from_number, to_number, content, direction, status, created_at
FROM messages
WHERE company_id = $1 AND charge_id = $2 AND direction = 'inbound'
ORDER BY created_at DESC, id DESC
`
	rows, err := r.db.QueryContext(ctx, q, companyID, chargeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Message
	for rows.Next() {
		var m models.Message
		var charge sql.NullInt64
		if err := rows.Scan(&m.ID, &m.CompanyID, &charge, &m.FromNumber, &m.ToNumber, &m.Content, &m.Direction, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		if charge.Valid {
			v := charge.Int64
			m.ChargeID = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// InsertMessage guarda un mensaje de WhatsApp.
func (r *Repository) InsertMessage(ctx context.Context, m *models.Message) (int64, error) {
	var chargeID any
	if m.ChargeID != nil {
		chargeID = *m.ChargeID
	} else {
		chargeID = nil
	}
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO messages (company_id, charge_id, from_number, to_number, content, direction, status) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		m.CompanyID, chargeID, m.FromNumber, m.ToNumber, m.Content, m.Direction, m.Status,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}
