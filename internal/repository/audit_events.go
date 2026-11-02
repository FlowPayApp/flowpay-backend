package repository

import (
	"context"

	"github.com/flowpay/flowpay-backend/internal/audit"
)

func (db *DB) EnsureAuditEvents(ctx context.Context) error {
	for _, q := range audit.SchemaStatements() {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) RecordAudit(ctx context.Context, e audit.Event) error {
	return audit.Insert(ctx, db.db, e)
}
