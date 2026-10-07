package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrCompanyMissing la empresa no existe en companies.
	ErrCompanyMissing = errors.New("company missing")
	// ErrWhatsAppNumberTaken el número activo ya pertenece a otra empresa.
	ErrWhatsAppNumberTaken = errors.New("whatsapp number taken")
)

// FindWhatsAppNumberByTo busca el tenant por el número receptor (normalizado whatsapp:+...).
func (db *DB) FindWhatsAppNumberByTo(ctx context.Context, toNormalized string) (*model.WhatsAppNumber, error) {
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
	var w model.WhatsAppNumber
	err := db.db.QueryRowContext(ctx, q, toNormalized).Scan(
		&w.ID, &w.CompanyID, &w.PhoneNumber, &w.TwilioSID, &w.Status, &w.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// FirstActiveWhatsAppToForCompany primer número Twilio activo de la empresa (receptor en webhooks).
func (db *DB) FirstActiveWhatsAppToForCompany(ctx context.Context, companyID int64) (string, error) {
	var phone string
	err := db.db.QueryRowContext(ctx, `
SELECT phone_number FROM whatsapp_numbers
WHERE company_id = $1 AND status = 'active'
ORDER BY id ASC LIMIT 1
`, companyID).Scan(&phone)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(phone), nil
}

// ListActiveWhatsAppNumbers números Business activos, uno por empresa.
func (db *DB) ListActiveWhatsAppNumbers(ctx context.Context) ([]model.WhatsAppNumber, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT id, company_id, phone_number, twilio_sid, status, created_at
FROM whatsapp_numbers
WHERE status = 'active'
ORDER BY company_id ASC, id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.WhatsAppNumber, 0)
	for rows.Next() {
		var w model.WhatsAppNumber
		if err := rows.Scan(&w.ID, &w.CompanyID, &w.PhoneNumber, &w.TwilioSID, &w.Status, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetCompanyWhatsAppNumber deja un único número activo para la empresa.
// phone vacío desasigna. El número debe ir normalizado (whatsapp:+...).
func (db *DB) SetCompanyWhatsAppNumber(ctx context.Context, companyID int64, phone string) (*model.WhatsAppNumber, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM companies WHERE id = $1)`, companyID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrCompanyMissing
	}

	phone = strings.TrimSpace(phone)
	if phone != "" {
		var other int64
		err := tx.QueryRowContext(ctx, `
SELECT company_id FROM whatsapp_numbers
WHERE status = 'active' AND LOWER(phone_number) = LOWER($1) AND company_id <> $2
LIMIT 1
`, phone, companyID).Scan(&other)
		if err == nil {
			return nil, ErrWhatsAppNumberTaken
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE whatsapp_numbers SET status = 'inactive'
WHERE company_id = $1 AND status = 'active'
`, companyID); err != nil {
		return nil, err
	}

	if phone == "" {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	var w model.WhatsAppNumber
	err = tx.QueryRowContext(ctx, `
INSERT INTO whatsapp_numbers (company_id, phone_number, twilio_sid, status)
VALUES ($1, $2, '', 'active')
RETURNING id, company_id, phone_number, twilio_sid, status, created_at
`, companyID, phone).Scan(&w.ID, &w.CompanyID, &w.PhoneNumber, &w.TwilioSID, &w.Status, &w.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrWhatsAppNumberTaken
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &w, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// GetMessageByID mensaje por id y empresa (cualquier dirección).
func (db *DB) GetMessageByID(ctx context.Context, companyID, msgID int64) (*model.Message, error) {
	q := `
SELECT id, company_id, charge_id, from_number, to_number, content, direction, status, created_at
FROM messages WHERE id = $1 AND company_id = $2
`
	var m model.Message
	var charge sql.NullInt64
	err := db.db.QueryRowContext(ctx, q, msgID, companyID).Scan(
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
func (db *DB) FindOpenChargeIDForInboundWhatsApp(ctx context.Context, companyID int64, fromNormalized string) (*int64, error) {
	q := `
SELECT ch.id, COALESCE(c.phone, '') AS phone
FROM charges ch
JOIN clients c ON c.id = ch.client_id
WHERE ch.company_id = $1
  AND ch.paid_at IS NULL
  AND c.phone IS NOT NULL AND TRIM(c.phone) <> ''
ORDER BY ch.created_at DESC, ch.id DESC
`
	rows, err := db.db.QueryContext(ctx, q, companyID)
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
func (db *DB) ListInboundMessagesForCharge(ctx context.Context, companyID, chargeID int64) ([]model.Message, error) {
	q := `
SELECT id, company_id, charge_id, from_number, to_number, content, direction, status, created_at
FROM messages
WHERE company_id = $1 AND charge_id = $2 AND direction = 'inbound'
ORDER BY created_at DESC, id DESC
`
	rows, err := db.db.QueryContext(ctx, q, companyID, chargeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Message
	for rows.Next() {
		var m model.Message
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
func (db *DB) InsertMessage(ctx context.Context, m *model.Message) (int64, error) {
	var chargeID any
	if m.ChargeID != nil {
		chargeID = *m.ChargeID
	} else {
		chargeID = nil
	}
	var id int64
	err := db.db.QueryRowContext(ctx,
		`INSERT INTO messages (company_id, charge_id, from_number, to_number, content, direction, status) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		m.CompanyID, chargeID, m.FromNumber, m.ToNumber, m.Content, m.Direction, m.Status,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}
