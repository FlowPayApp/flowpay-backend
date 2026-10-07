package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/model"
)

// ListCompanyMailboxes buzones configurados, uno por empresa.
func (db *DB) ListCompanyMailboxes(ctx context.Context) ([]model.CompanyMailbox, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT id, company_id, from_name, from_email, smtp_host, smtp_port, smtp_username, smtp_password, updated_at
FROM company_mailboxes
ORDER BY company_id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.CompanyMailbox, 0)
	for rows.Next() {
		m, err := scanMailbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetCompanyMailbox buzón de una empresa. sql.ErrNoRows si no tiene.
func (db *DB) GetCompanyMailbox(ctx context.Context, companyID int64) (*model.CompanyMailbox, error) {
	row := db.db.QueryRowContext(ctx, `
SELECT id, company_id, from_name, from_email, smtp_host, smtp_port, smtp_username, smtp_password, updated_at
FROM company_mailboxes
WHERE company_id = $1
`, companyID)
	m, err := scanMailbox(row)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpsertCompanyMailbox crea o reemplaza el buzón. password vacío conserva la anterior.
// fromEmail vacío borra el buzón.
func (db *DB) UpsertCompanyMailbox(ctx context.Context, m model.CompanyMailbox) (*model.CompanyMailbox, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM companies WHERE id = $1)`, m.CompanyID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrCompanyMissing
	}

	if strings.TrimSpace(m.FromEmail) == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM company_mailboxes WHERE company_id = $1`, m.CompanyID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	var saved model.CompanyMailbox
	err = tx.QueryRowContext(ctx, `
INSERT INTO company_mailboxes (company_id, from_name, from_email, smtp_host, smtp_port, smtp_username, smtp_password, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
ON CONFLICT (company_id) DO UPDATE SET
	from_name = EXCLUDED.from_name,
	from_email = EXCLUDED.from_email,
	smtp_host = EXCLUDED.smtp_host,
	smtp_port = EXCLUDED.smtp_port,
	smtp_username = EXCLUDED.smtp_username,
	smtp_password = CASE WHEN EXCLUDED.smtp_password = '' THEN company_mailboxes.smtp_password ELSE EXCLUDED.smtp_password END,
	updated_at = NOW()
RETURNING id, company_id, from_name, from_email, smtp_host, smtp_port, smtp_username, smtp_password, updated_at
`, m.CompanyID, m.FromName, m.FromEmail, m.SMTPHost, m.SMTPPort, m.SMTPUsername, m.SMTPPassword).Scan(
		&saved.ID, &saved.CompanyID, &saved.FromName, &saved.FromEmail, &saved.SMTPHost, &saved.SMTPPort, &saved.SMTPUsername, &saved.SMTPPassword, &saved.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrMailboxTaken
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	saved.PasswordSet = strings.TrimSpace(saved.SMTPPassword) != ""
	saved.SMTPPassword = ""
	return &saved, nil
}

// ErrMailboxTaken el correo ya pertenece a otra empresa.
var ErrMailboxTaken = errors.New("mailbox taken")

type mailboxScanner interface {
	Scan(dest ...any) error
}

func scanMailbox(s mailboxScanner) (model.CompanyMailbox, error) {
	var m model.CompanyMailbox
	err := s.Scan(&m.ID, &m.CompanyID, &m.FromName, &m.FromEmail, &m.SMTPHost, &m.SMTPPort, &m.SMTPUsername, &m.SMTPPassword, &m.UpdatedAt)
	if err != nil {
		return model.CompanyMailbox{}, err
	}
	m.PasswordSet = strings.TrimSpace(m.SMTPPassword) != ""
	return m, nil
}
