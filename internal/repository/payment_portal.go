package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// PaymentPortal es el enlace que la empresa quiere en los recordatorios.
// Mode external usa URL. PendingURL espera la revisión de la plataforma.
type PaymentPortal struct {
	Mode       string
	URL        string
	PendingURL string
	PendingAt  *time.Time
	Rejection  string
}

// PendingPaymentPortal es una dirección propia todavía sin aprobar.
type PendingPaymentPortal struct {
	CompanyID   int64
	CompanyName string
	Mode        string
	URL         string
	PendingURL  string
	PendingAt   *time.Time
}

// ListPlatformAdminEmails correos de los superadmin activos.
func (db *DB) ListPlatformAdminEmails(ctx context.Context) ([]string, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT email FROM users
WHERE is_platform_admin = TRUE AND is_active = TRUE AND email <> ''
ORDER BY id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		email = strings.TrimSpace(email)
		if email != "" {
			out = append(out, email)
		}
	}
	return out, rows.Err()
}

// EnsurePaymentPortalSchema guarda el portal elegido y la dirección pendiente de aprobación.
func (db *DB) EnsurePaymentPortalSchema(ctx context.Context) error {
	stmts := []string{
		`ALTER TABLE companies ADD COLUMN IF NOT EXISTS payment_portal TEXT NOT NULL DEFAULT 'flowpay'`,
		`ALTER TABLE companies ADD COLUMN IF NOT EXISTS payment_portal_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE companies ADD COLUMN IF NOT EXISTS payment_portal_pending_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE companies ADD COLUMN IF NOT EXISTS payment_portal_pending_at TIMESTAMPTZ NULL`,
		`ALTER TABLE companies ADD COLUMN IF NOT EXISTS payment_portal_rejection TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_companies_payment_portal_pending
			ON companies (payment_portal_pending_at)
			WHERE payment_portal_pending_url <> ''`,
	}
	for _, q := range stmts {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) GetPaymentPortal(ctx context.Context, companyID int64) (PaymentPortal, error) {
	var p PaymentPortal
	var pendingAt sql.NullTime
	err := db.db.QueryRowContext(ctx, `
SELECT COALESCE(payment_portal, 'flowpay'),
       COALESCE(payment_portal_url, ''),
       COALESCE(payment_portal_pending_url, ''),
       payment_portal_pending_at,
       COALESCE(payment_portal_rejection, '')
FROM companies WHERE id = $1
`, companyID).Scan(&p.Mode, &p.URL, &p.PendingURL, &pendingAt, &p.Rejection)
	if err != nil {
		return PaymentPortal{}, err
	}
	if pendingAt.Valid {
		t := pendingAt.Time
		p.PendingAt = &t
	}
	return p, nil
}

// SavePaymentPortal reemplaza la elección. Una pendiente nueva renueva la fecha; la misma la conserva.
func (db *DB) SavePaymentPortal(ctx context.Context, companyID int64, p PaymentPortal) error {
	res, err := db.db.ExecContext(ctx, `
UPDATE companies
SET payment_portal = $1,
    payment_portal_url = $2,
    payment_portal_pending_url = $3,
    payment_portal_pending_at = CASE
        WHEN $3 = '' THEN NULL
        WHEN $3 IS DISTINCT FROM payment_portal_pending_url THEN NOW()
        ELSE payment_portal_pending_at
    END,
    payment_portal_rejection = $4
WHERE id = $5
`, p.Mode, p.URL, p.PendingURL, p.Rejection, companyID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) ListPendingPaymentPortals(ctx context.Context) ([]PendingPaymentPortal, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT id, name,
       COALESCE(payment_portal, 'flowpay'),
       COALESCE(payment_portal_url, ''),
       payment_portal_pending_url,
       payment_portal_pending_at
FROM companies
WHERE payment_portal_pending_url <> ''
ORDER BY payment_portal_pending_at NULLS LAST, id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingPaymentPortal
	for rows.Next() {
		var item PendingPaymentPortal
		var pendingAt sql.NullTime
		if err := rows.Scan(&item.CompanyID, &item.CompanyName, &item.Mode, &item.URL, &item.PendingURL, &pendingAt); err != nil {
			return nil, err
		}
		if pendingAt.Valid {
			t := pendingAt.Time
			item.PendingAt = &t
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ApprovePaymentPortal pasa la dirección pendiente a la que usan los recordatorios.
func (db *DB) ApprovePaymentPortal(ctx context.Context, companyID int64) error {
	res, err := db.db.ExecContext(ctx, `
UPDATE companies
SET payment_portal = 'external',
    payment_portal_url = payment_portal_pending_url,
    payment_portal_pending_url = '',
    payment_portal_pending_at = NULL,
    payment_portal_rejection = ''
WHERE id = $1 AND payment_portal_pending_url <> ''
`, companyID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RejectPaymentPortal descarta la pendiente y guarda el motivo. La dirección en uso no cambia.
func (db *DB) RejectPaymentPortal(ctx context.Context, companyID int64, reason string) error {
	res, err := db.db.ExecContext(ctx, `
UPDATE companies
SET payment_portal_pending_url = '',
    payment_portal_pending_at = NULL,
    payment_portal_rejection = $2
WHERE id = $1 AND payment_portal_pending_url <> ''
`, companyID, reason)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
