package repository

import (
	"context"
	"database/sql"
)

type sqlExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// PaymentCredentialFlags indica qué medios tienen secreto guardado. No incluye el valor.
// TransferGateway queda guardado. El cliente ve Khipu y Fintoc cuando las dos credenciales están cargadas.
type PaymentCredentialFlags struct {
	Khipu           bool
	Webpay          bool
	Oneclick        bool
	Fintoc          bool
	TransferGateway string
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
	"fintoc_webhook_secret",
}

func (db *DB) EnsurePaymentCredentialsSchema(ctx context.Context) error {
	const createTable = `
CREATE TABLE IF NOT EXISTS company_payment_credentials (
	company_id BIGINT PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
	khipu_api_key TEXT,
	webpay_commerce_code TEXT,
	webpay_api_key TEXT,
	oneclick_mall_code TEXT,
	oneclick_store_code TEXT,
	oneclick_api_key TEXT,
	fintoc_secret_key TEXT,
	fintoc_webhook_secret TEXT,
	transfer_gateway TEXT,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`
	if _, err := db.db.ExecContext(ctx, createTable); err != nil {
		return err
	}
	const addColumns = `
ALTER TABLE company_payment_credentials
	ADD COLUMN IF NOT EXISTS fintoc_webhook_secret TEXT,
	ADD COLUMN IF NOT EXISTS transfer_gateway TEXT`
	if _, err := db.db.ExecContext(ctx, addColumns); err != nil {
		return err
	}
	const confirmations = `
CREATE TABLE IF NOT EXISTS payment_credential_confirmations (
	id BIGSERIAL PRIMARY KEY,
	company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL UNIQUE,
	provider TEXT NOT NULL,
	action TEXT NOT NULL,
	admin_email TEXT NOT NULL,
	payload TEXT NOT NULL,
	requested_by BIGINT,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`
	_, err := db.db.ExecContext(ctx, confirmations)
	return err
}

func (db *DB) PaymentCredentialFlags(ctx context.Context, companyID int64) (PaymentCredentialFlags, error) {
	var flags PaymentCredentialFlags
	var gateway sql.NullString
	err := db.db.QueryRowContext(ctx, `
SELECT
	khipu_api_key IS NOT NULL,
	webpay_commerce_code IS NOT NULL AND webpay_api_key IS NOT NULL,
	oneclick_mall_code IS NOT NULL AND oneclick_store_code IS NOT NULL AND oneclick_api_key IS NOT NULL,
	fintoc_secret_key IS NOT NULL,
	transfer_gateway
FROM company_payment_credentials
WHERE company_id = $1
`, companyID).Scan(&flags.Khipu, &flags.Webpay, &flags.Oneclick, &flags.Fintoc, &gateway)
	if gateway.Valid {
		flags.TransferGateway = gateway.String
	}
	if err == sql.ErrNoRows {
		return PaymentCredentialFlags{}, nil
	}
	return flags, err
}

// SavePaymentCredentialFields escribe solo las columnas presentes. Un nil borra el secreto.
func (db *DB) SavePaymentCredentialFields(ctx context.Context, companyID int64, fields map[string]*string) error {
	return savePaymentCredentialFields(ctx, db.db, companyID, fields)
}

func savePaymentCredentialFields(ctx context.Context, ex sqlExec, companyID int64, fields map[string]*string) error {
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
	_, err := ex.ExecContext(ctx, query, args...)
	return err
}

// SetTransferGateway guarda khipu o fintoc. No oculta la otra transferencia en el enlace.
func (db *DB) SetTransferGateway(ctx context.Context, companyID int64, gateway string) error {
	_, err := db.db.ExecContext(ctx, `
INSERT INTO company_payment_credentials (company_id, transfer_gateway, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (company_id) DO UPDATE SET transfer_gateway = EXCLUDED.transfer_gateway, updated_at = NOW()
`, companyID, gateway)
	return err
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
