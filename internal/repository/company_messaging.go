package repository

import (
	"context"
	"database/sql"
)

// CompanyMessaging textos globales para armar recordatorios (placeholders en plantillas).
type CompanyMessaging struct {
	Name                  string  `json:"name"`
	TransferInstructions  string  `json:"transfer_instructions"`
	PaymentURLTemplate    string  `json:"payment_url_template"`
}

func (r *Repository) GetCompanyMessaging(ctx context.Context, companyID int64) (*CompanyMessaging, error) {
	var m CompanyMessaging
	var ti, pu sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT name, transfer_instructions, payment_url_template
FROM companies WHERE id = ?
`, companyID).Scan(&m.Name, &ti, &pu)
	if err != nil {
		return nil, err
	}
	if ti.Valid {
		m.TransferInstructions = ti.String
	}
	if pu.Valid {
		m.PaymentURLTemplate = pu.String
	}
	return &m, nil
}

func (r *Repository) UpdateCompanyMessaging(ctx context.Context, companyID int64, transferInstructions, paymentURLTemplate string) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE companies SET transfer_instructions = ?, payment_url_template = ? WHERE id = ?
`, nullStr(transferInstructions), nullStr(paymentURLTemplate), companyID)
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

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
