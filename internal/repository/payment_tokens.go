package repository

import (
	"context"
	"database/sql"
	"time"
)

// PaymentTokenRow fila persistida de token de pago.
type PaymentTokenRow struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"company_id"`
	ClientID  int64     `json:"client_id"`
	Token     string    `json:"token"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// InsertPaymentToken inserta un token de pago. company_id y client_id deben ser coherentes (FK).
func (r *Repository) InsertPaymentToken(ctx context.Context, companyID, clientID int64, token, status string) (*PaymentTokenRow, error) {
	var row PaymentTokenRow
	err := r.db.QueryRowContext(ctx, `
INSERT INTO payment_tokens (company_id, client_id, token, status)
VALUES ($1, $2, $3, $4)
RETURNING id, company_id, client_id, token, status, created_at
`, companyID, clientID, token, status).Scan(
		&row.ID, &row.CompanyID, &row.ClientID, &row.Token, &row.Status, &row.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// GetPaymentTokenByValue devuelve la fila por valor de token (para resolución pública más adelante).
func (r *Repository) GetPaymentTokenByValue(ctx context.Context, token string) (*PaymentTokenRow, error) {
	var row PaymentTokenRow
	err := r.db.QueryRowContext(ctx, `
SELECT id, company_id, client_id, token, status, created_at
FROM payment_tokens
WHERE trim(both from token) = trim(both from $1::text)
LIMIT 1
`, token).Scan(&row.ID, &row.CompanyID, &row.ClientID, &row.Token, &row.Status, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ErrPaymentTokenNotFound cuando no existe fila para el token dado.
func ErrPaymentTokenNotFound(err error) bool {
	return err == sql.ErrNoRows
}

// MarkPaymentTokenViewed pasa el token de 'issued' a 'viewed'.
// Solo aplica si está en 'issued' (no degrada estados posteriores como 'paid' o 'revoked').
// Devuelve true si efectivamente cambió.
func (r *Repository) MarkPaymentTokenViewed(ctx context.Context, token string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
UPDATE payment_tokens
SET status = 'viewed'::payment_token_status
WHERE trim(both from token) = trim(both from $1::text)
  AND status = 'issued'::payment_token_status
`, token)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
