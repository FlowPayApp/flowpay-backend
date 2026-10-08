package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

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

type storedMedia struct {
	URL         string `json:"url,omitempty"`
	FileToken   string `json:"file_token,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	ContentType string `json:"content_type"`
}

func encodeMedia(media []model.MessageMedia) (string, error) {
	out := make([]storedMedia, 0, len(media))
	for _, m := range media {
		out = append(out, storedMedia{URL: m.URL, FileToken: m.FileToken, FileName: m.FileName, ContentType: m.ContentType})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeMedia(raw []byte) []model.MessageMedia {
	out := []model.MessageMedia{}
	var stored []storedMedia
	if len(raw) == 0 || json.Unmarshal(raw, &stored) != nil {
		return out
	}
	for _, m := range stored {
		out = append(out, model.MessageMedia{URL: m.URL, FileToken: m.FileToken, FileName: m.FileName, ContentType: m.ContentType})
	}
	return out
}

const messageColumns = `id, company_id, charge_id, from_number, to_number, content, media, direction, status, read_at, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessage(row rowScanner) (*model.Message, error) {
	var m model.Message
	var charge sql.NullInt64
	var readAt sql.NullTime
	var media []byte
	if err := row.Scan(&m.ID, &m.CompanyID, &charge, &m.FromNumber, &m.ToNumber, &m.Content, &media, &m.Direction, &m.Status, &readAt, &m.CreatedAt); err != nil {
		return nil, err
	}
	if charge.Valid {
		v := charge.Int64
		m.ChargeID = &v
	}
	if readAt.Valid {
		v := readAt.Time
		m.ReadAt = &v
	}
	m.Media = decodeMedia(media)
	return &m, nil
}

// GetMessageByID mensaje por id y empresa (cualquier dirección).
func (db *DB) GetMessageByID(ctx context.Context, companyID, msgID int64) (*model.Message, error) {
	q := `SELECT ` + messageColumns + ` FROM messages WHERE id = $1 AND company_id = $2`
	return scanMessage(db.db.QueryRowContext(ctx, q, msgID, companyID))
}

// MarkChargeInboundRead marca como leídas las respuestas del cobro. Devuelve cuántas cambió.
func (db *DB) MarkChargeInboundRead(ctx context.Context, companyID, chargeID int64) (int64, error) {
	res, err := db.db.ExecContext(ctx, `
UPDATE messages SET read_at = NOW()
WHERE company_id = $1 AND charge_id = $2 AND direction = 'inbound' AND read_at IS NULL
`, companyID, chargeID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// unreadScope respuestas sin leer de cobros que el usuario puede ver (los cobradores, solo su cartera).
const unreadScope = `
FROM messages m
JOIN charges i ON i.id = m.charge_id AND i.company_id = m.company_id
JOIN clients c ON c.id = i.client_id
WHERE m.company_id = $1 AND m.direction = 'inbound' AND m.read_at IS NULL
  AND ($2::bigint = 0 OR COALESCE(c.assigned_to, c.created_by) = $2)
`

// ListUnreadInbound cobros con respuestas sin leer, el más reciente primero.
func (db *DB) ListUnreadInbound(ctx context.Context, companyID, memberUID int64, limit int) (*model.Inbox, error) {
	out := &model.Inbox{Threads: []model.UnreadThread{}}
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) `+unreadScope, companyID, memberUID).Scan(&out.Total); err != nil {
		return nil, err
	}
	if out.Total == 0 {
		return out, nil
	}
	rows, err := db.db.QueryContext(ctx, `
SELECT i.id, `+chargeClientLabelExpr+`, COUNT(*), MAX(m.created_at),
       (ARRAY_AGG(m.content ORDER BY m.created_at DESC, m.id DESC))[1],
       (ARRAY_AGG(jsonb_array_length(m.media) > 0 ORDER BY m.created_at DESC, m.id DESC))[1]
`+unreadScope+`
GROUP BY i.id, c.id
ORDER BY MAX(m.created_at) DESC
LIMIT $3
`, companyID, memberUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t model.UnreadThread
		if err := rows.Scan(&t.ChargeID, &t.ClientName, &t.Unread, &t.LastAt, &t.Preview, &t.HasMedia); err != nil {
			return nil, err
		}
		out.Threads = append(out.Threads, t)
	}
	return out, rows.Err()
}

// InsertMessageFile guarda un archivo enviado por la empresa.
func (db *DB) InsertMessageFile(ctx context.Context, companyID int64, token string, f model.MessageFile) error {
	_, err := db.db.ExecContext(ctx, `
INSERT INTO message_files (company_id, token, content_type, file_name, data) VALUES ($1, $2, $3, $4, $5)
`, companyID, token, f.ContentType, f.FileName, f.Data)
	return err
}

// DeleteMessageFile borra un archivo que no llegó a enviarse.
func (db *DB) DeleteMessageFile(ctx context.Context, companyID int64, token string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM message_files WHERE company_id = $1 AND token = $2`, companyID, token)
	return err
}

// GetMessageFile archivo de la empresa por token (para mostrarlo en el panel).
func (db *DB) GetMessageFile(ctx context.Context, companyID int64, token string) (*model.MessageFile, error) {
	var f model.MessageFile
	err := db.db.QueryRowContext(ctx, `
SELECT content_type, file_name, data FROM message_files WHERE company_id = $1 AND token = $2
`, companyID, token).Scan(&f.ContentType, &f.FileName, &f.Data)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// GetRecentMessageFile archivo por token sin sesión, solo mientras Twilio lo descarga para entregarlo.
func (db *DB) GetRecentMessageFile(ctx context.Context, token string, maxAge time.Duration) (*model.MessageFile, error) {
	var f model.MessageFile
	err := db.db.QueryRowContext(ctx, `
SELECT content_type, file_name, data FROM message_files
WHERE token = $1 AND created_at > NOW() - make_interval(secs => $2)
`, token, maxAge.Seconds()).Scan(&f.ContentType, &f.FileName, &f.Data)
	if err != nil {
		return nil, err
	}
	return &f, nil
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

// ListInboundMessagesForCharge mensajes de WhatsApp del cobro: respuestas del cliente y textos enviados desde la ficha.
func (db *DB) ListInboundMessagesForCharge(ctx context.Context, companyID, chargeID int64) ([]model.Message, error) {
	q := `SELECT ` + messageColumns + `
FROM messages
WHERE company_id = $1 AND charge_id = $2 AND direction IN ('inbound', 'outbound')
ORDER BY created_at DESC, id DESC
`
	rows, err := db.db.QueryContext(ctx, q, companyID, chargeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// HasInboundFromPhoneSince indica si ese teléfono escribió a la empresa desde since.
func (db *DB) HasInboundFromPhoneSince(ctx context.Context, companyID int64, phone string, since time.Time) (bool, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT from_number
FROM messages
WHERE company_id = $1 AND direction = 'inbound' AND created_at >= $2
ORDER BY created_at DESC
LIMIT 300
`, companyID, since)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var from string
		if err := rows.Scan(&from); err != nil {
			return false, err
		}
		if phonesLikelyMatch(from, phone) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// InsertMessage guarda un mensaje de WhatsApp.
func (db *DB) InsertMessage(ctx context.Context, m *model.Message) (int64, error) {
	var chargeID any
	if m.ChargeID != nil {
		chargeID = *m.ChargeID
	} else {
		chargeID = nil
	}
	media, err := encodeMedia(m.Media)
	if err != nil {
		return 0, err
	}
	var id int64
	err = db.db.QueryRowContext(ctx,
		`INSERT INTO messages (company_id, charge_id, from_number, to_number, content, media, direction, status) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8) RETURNING id`,
		m.CompanyID, chargeID, m.FromNumber, m.ToNumber, m.Content, media, m.Direction, m.Status,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}
