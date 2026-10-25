package repository

import (
	"context"
	"database/sql"
)

// PaymentCredentialFlags indica qué medios tienen secreto guardado. No incluye el valor.
type PaymentCredentialFlags struct {
	Khipu    bool
	Webpay   bool
	Oneclick bool
	Fintoc   bool
}

// PaymentCredentialColumns son las únicas columnas que se pueden escribir.
var PaymentCredentialColumns = []string{
	"khipu_api_key",
	"webpay_commerce_code",
	"webpay_api_key",
	"oneclick_mall_code",
	"oneclick_store_code",
	"oneclick_api_key",
	"fintoc_secret_key",
}

func (db *DB) EnsurePaymentCredentialsSchema(ctx context.Context) error {
	_, err := db.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS company_payment_credentials (
	company_id BIGINT PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
	khipu_api_key TEXT,
	webpay_commerce_code TEXT,
	webpay_api_key TEXT,
	oneclick_mall_code TEXT,
	oneclick_store_code TEXT,
	oneclick_api_key TEXT,
	fintoc_secret_key TEXT,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
	return err
}

func (db *DB) PaymentCredentialFlags(ctx context.Context, companyID int64) (PaymentCredentialFlags, error) {
	var flags PaymentCredentialFlags
	err := db.db.QueryRowContext(ctx, `
SELECT
	khipu_api_key IS NOT NULL,
	webpay_commerce_code IS NOT NULL AND webpay_api_key IS NOT NULL,
	oneclick_mall_code IS NOT NULL AND oneclick_store_code IS NOT NULL AND oneclick_api_key IS NOT NULL,
	fintoc_secret_key IS NOT NULL
FROM company_payment_credentials
WHERE company_id = $1
`, companyID).Scan(&flags.Khipu, &flags.Webpay, &flags.Oneclick, &flags.Fintoc)
	if err == sql.ErrNoRows {
		return PaymentCredentialFlags{}, nil
	}
	return flags, err
}

// SavePaymentCredentialFields escribe solo las columnas presentes. Un nil borra el secreto.
func (db *DB) SavePaymentCredentialFields(ctx context.Context, companyID int64, fields map[string]*string) error {
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	args = append(args, companyID)
	for _, name := range PaymentCredentialColumns {
		value, ok := fields[name]
		if !ok {
			continue
		}
		cols = append(cols, name)
		if value == nil || *value == "" {
			args = append(args, nil)
			continue
		}
		args = append(args, *value)
	}
	if len(cols) == 0 {
		return nil
	}
	query := "INSERT INTO company_payment_credentials (company_id"
	values := "VALUES ($1"
	updates := "updated_at = NOW()"
	for i, name := range cols {
		query += ", " + name
		values += ", $" + itoa(i+2)
		updates += ", " + name + " = EXCLUDED." + name
	}
	query += ", updated_at) " + values + ", NOW()) ON CONFLICT (company_id) DO UPDATE SET " + updates
	_, err := db.db.ExecContext(ctx, query, args...)
	return err
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
