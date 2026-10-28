package repository

import "context"

// InsertAudit anota quién hizo qué, en qué empresa. target 0 queda NULL.
func (db *DB) InsertAudit(ctx context.Context, actor, target, company int64, action string) error {
	_, err := db.db.ExecContext(ctx, `
INSERT INTO audit_log (actor_user_id, target_id, company_id, action)
VALUES (NULLIF($1, 0), NULLIF($2, 0), NULLIF($3, 0), $4)`,
		actor, target, company, action)
	return err
}
