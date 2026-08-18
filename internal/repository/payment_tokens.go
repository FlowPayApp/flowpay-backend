package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// LatestPaymentTokenForClient token de portal vigente (issued/viewed de preferencia) del cliente.
func (db *DB) LatestPaymentTokenForClient(ctx context.Context, companyID, clientID int64) (string, error) {
	var token string

	err := db.db.QueryRowContext(ctx, `
SELECT token
FROM payment_tokens
WHERE company_id = $1
  AND client_id = $2
  AND status::text <> 'revoked'
ORDER BY
  CASE WHEN status::text IN ('issued', 'viewed') THEN 0 ELSE 1 END,
  created_at DESC,
  id DESC
LIMIT 1
`, companyID, clientID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}
