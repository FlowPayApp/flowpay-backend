package repository

import (
	"context"
	"time"
)

func (db *DB) ListLiveRevokedJTIs(ctx context.Context) (map[string]time.Time, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT jti, expires_at FROM revoked_access_tokens WHERE expires_at > NOW()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var jti string
		var exp time.Time
		if err := rows.Scan(&jti, &exp); err != nil {
			return nil, err
		}
		if jti != "" {
			out[jti] = exp
		}
	}
	return out, rows.Err()
}
