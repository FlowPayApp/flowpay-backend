package repository

import "context"

// EnsureClientPortfolioColumns alinea columnas de cartera usadas para filtrar cobros.
func (db *DB) EnsureClientPortfolioColumns(ctx context.Context) error {
	stmts := []string{
		`ALTER TABLE clients ADD COLUMN IF NOT EXISTS created_by BIGINT NULL`,
		`ALTER TABLE clients ADD COLUMN IF NOT EXISTS assigned_to BIGINT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_clients_company_assigned_to ON clients (company_id, assigned_to)`,
		`CREATE INDEX IF NOT EXISTS idx_clients_company_created_by ON clients (company_id, created_by)`,
	}
	for _, q := range stmts {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// EnsureReminderTemplateColumns separa el texto de WhatsApp del cuerpo del correo.
func (db *DB) EnsureReminderTemplateColumns(ctx context.Context) error {
	stmts := []string{
		`ALTER TABLE company_reminder_templates ADD COLUMN IF NOT EXISTS whatsapp_body TEXT`,
		`UPDATE company_reminder_templates SET whatsapp_body = body WHERE whatsapp_body IS NULL`,
		`ALTER TABLE company_reminder_templates ALTER COLUMN whatsapp_body SET DEFAULT ''`,
		`ALTER TABLE company_reminder_templates ALTER COLUMN whatsapp_body SET NOT NULL`,
	}
	for _, q := range stmts {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// EnsureMailboxSchema guarda el buzón SMTP con el que cada empresa envía recordatorios.
func (db *DB) EnsureMailboxSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS company_mailboxes (
			id BIGSERIAL PRIMARY KEY,
			company_id BIGINT NOT NULL,
			from_name TEXT NOT NULL DEFAULT '',
			from_email TEXT NOT NULL,
			smtp_host TEXT NOT NULL,
			smtp_port TEXT NOT NULL DEFAULT '587',
			smtp_username TEXT NOT NULL,
			smtp_password TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_company_mailboxes_company ON company_mailboxes (company_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_company_mailboxes_from_email ON company_mailboxes (LOWER(from_email))`,
	}
	for _, q := range stmts {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// EnsureWhatsAppSchema crea las tablas que enrutan el WhatsApp Business de cada empresa.
func (db *DB) EnsureWhatsAppSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS whatsapp_numbers (
			id BIGSERIAL PRIMARY KEY,
			company_id BIGINT NOT NULL,
			phone_number TEXT NOT NULL,
			twilio_sid TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE whatsapp_numbers ADD COLUMN IF NOT EXISTS twilio_sid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE whatsapp_numbers ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE whatsapp_numbers ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_whatsapp_numbers_active_phone
			ON whatsapp_numbers (LOWER(phone_number))
			WHERE status = 'active'`,
		// El UNIQUE original de phone_number bloqueaba reasignar un número ya desactivado.
		`ALTER TABLE whatsapp_numbers DROP CONSTRAINT IF EXISTS whatsapp_numbers_phone_number_key`,
		`CREATE INDEX IF NOT EXISTS idx_whatsapp_numbers_company_active
			ON whatsapp_numbers (company_id)
			WHERE status = 'active'`,
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGSERIAL PRIMARY KEY,
			company_id BIGINT NOT NULL,
			charge_id BIGINT,
			from_number TEXT NOT NULL DEFAULT '',
			to_number TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			direction TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'received',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_company_charge
			ON messages (company_id, charge_id, created_at DESC)`,
	}
	for _, q := range stmts {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
