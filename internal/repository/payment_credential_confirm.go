package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrCredentialConfirmMissing = errors.New("este enlace ya no está vigente")
	ErrCredentialConfirmExpired = errors.New("este enlace caducó")
	ErrCredentialConfirmUsed    = errors.New("este cambio ya se confirmó")
)

// CredentialConfirmation es lo que el administrador ve antes de aceptar. No incluye secretos.
type CredentialConfirmation struct {
	CompanyName string
	Provider    string
	Action      string
	AdminEmail  string
	ExpiresAt   time.Time
	Used        bool
}

// PendingCredentialConfirmation es un cambio que todavía espera el correo.
type PendingCredentialConfirmation struct {
	Provider   string
	Action     string
	AdminEmail string
	ExpiresAt  time.Time
}

// CompanyRegisteredAdmin es el primer administrador activo de la empresa.
type CompanyRegisteredAdmin struct {
	Email       string
	Name        string
	CompanyName string
}

func (db *DB) CompanyRegisteredAdmin(ctx context.Context, companyID int64) (CompanyRegisteredAdmin, error) {
	var admin CompanyRegisteredAdmin
	err := db.db.QueryRowContext(ctx, `
SELECT COALESCE(u.email, ''), COALESCE(u.name, ''), COALESCE(c.name, '')
FROM companies c
JOIN company_users cu ON cu.company_id = c.id AND cu.role = 'admin'
JOIN users u ON u.id = cu.user_id AND u.is_platform_admin = FALSE AND u.is_active = TRUE
WHERE c.id = $1 AND COALESCE(u.email, '') <> ''
ORDER BY u.id ASC
LIMIT 1
`, companyID).Scan(&admin.Email, &admin.Name, &admin.CompanyName)
	if err == sql.ErrNoRows {
		return CompanyRegisteredAdmin{}, sql.ErrNoRows
	}
	return admin, err
}

func (db *DB) ReplaceCredentialConfirmation(ctx context.Context, companyID int64, hash, provider, action, adminEmail string, payload []byte, requestedBy int64, expires time.Time) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM payment_credential_confirmations
WHERE company_id = $1 AND provider = $2 AND used_at IS NULL
`, companyID, provider); err != nil {
		return err
	}
	var requested any
	if requestedBy > 0 {
		requested = requestedBy
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO payment_credential_confirmations
	(company_id, token_hash, provider, action, admin_email, payload, requested_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, companyID, hash, provider, action, adminEmail, string(payload), requested, expires); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) DeleteCredentialConfirmation(ctx context.Context, hash string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM payment_credential_confirmations WHERE token_hash = $1 AND used_at IS NULL`, hash)
	return err
}

func (db *DB) CredentialConfirmation(ctx context.Context, hash string) (CredentialConfirmation, error) {
	var row CredentialConfirmation
	var used sql.NullTime
	err := db.db.QueryRowContext(ctx, `
SELECT COALESCE(c.name, ''), p.provider, p.action, p.admin_email, p.expires_at, p.used_at
FROM payment_credential_confirmations p
JOIN companies c ON c.id = p.company_id
WHERE p.token_hash = $1
`, hash).Scan(&row.CompanyName, &row.Provider, &row.Action, &row.AdminEmail, &row.ExpiresAt, &used)
	if err == sql.ErrNoRows {
		return CredentialConfirmation{}, ErrCredentialConfirmMissing
	}
	if err != nil {
		return CredentialConfirmation{}, err
	}
	row.Used = used.Valid
	return row, nil
}

func (db *DB) ListPendingCredentialConfirmations(ctx context.Context, companyID int64, now time.Time) ([]PendingCredentialConfirmation, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT provider, action, admin_email, expires_at
FROM payment_credential_confirmations
WHERE company_id = $1 AND used_at IS NULL AND expires_at > $2
ORDER BY created_at DESC
`, companyID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingCredentialConfirmation
	for rows.Next() {
		var row PendingCredentialConfirmation
		if err := rows.Scan(&row.Provider, &row.Action, &row.AdminEmail, &row.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ConsumeCredentialConfirmation aplica el cambio cifrado y marca el enlace como usado.
func (db *DB) ConsumeCredentialConfirmation(ctx context.Context, hash string, now time.Time) (string, string, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()

	var companyID int64
	var provider, action, raw string
	var expires time.Time
	var used sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT company_id, provider, action, payload, expires_at, used_at
FROM payment_credential_confirmations
WHERE token_hash = $1
FOR UPDATE
`, hash).Scan(&companyID, &provider, &action, &raw, &expires, &used)
	if err == sql.ErrNoRows {
		return "", "", ErrCredentialConfirmMissing
	}
	if err != nil {
		return "", "", err
	}
	if used.Valid {
		return "", "", ErrCredentialConfirmUsed
	}
	if !expires.After(now) {
		return "", "", ErrCredentialConfirmExpired
	}
	var fields map[string]*string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return "", "", err
	}
	if err := savePaymentCredentialFields(ctx, tx, companyID, fields); err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE payment_credential_confirmations SET used_at = $2 WHERE token_hash = $1
`, hash, now); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return provider, action, nil
}
