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
