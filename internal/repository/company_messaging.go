package repository

import (
	"context"
	"database/sql"
)

// CompanyMessaging textos globales para armar recordatorios (placeholders en plantillas).
type CompanyMessaging struct {
	Name                 string `json:"name"`
	TransferInstructions string `json:"transfer_instructions"`
	PaymentURLTemplate   string `json:"payment_url_template"`
	PaymentPortal        string `json:"-"`
	PaymentPortalURL     string `json:"-"`
}

func (db *DB) GetCompanyMessaging(ctx context.Context, companyID int64) (*CompanyMessaging, error) {
	var m CompanyMessaging
	var ti, pu sql.NullString
	err := db.db.QueryRowContext(ctx, `
SELECT name, transfer_instructions, payment_url_template,
       COALESCE(payment_portal, 'flowpay'), COALESCE(payment_portal_url, '')
FROM companies WHERE id = $1
`, companyID).Scan(&m.Name, &ti, &pu, &m.PaymentPortal, &m.PaymentPortalURL)
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

func (db *DB) UpdateCompanyMessaging(ctx context.Context, companyID int64, transferInstructions, paymentURLTemplate string) error {
	res, err := db.db.ExecContext(ctx, `
UPDATE companies SET transfer_instructions = $1, payment_url_template = $2 WHERE id = $3
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

// CompanyNotifyAccess dice si el superadmin dejó activos el correo y WhatsApp.
// No depende de que el buzón o el número estén cargados.
func (db *DB) CompanyNotifyAccess(ctx context.Context, companyID int64) (email bool, whatsapp bool, err error) {
	err = db.db.QueryRowContext(ctx, `
SELECT COALESCE(allow_email, TRUE), COALESCE(allow_whatsapp, TRUE)
FROM companies WHERE id = $1
`, companyID).Scan(&email, &whatsapp)
	return email, whatsapp, err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
