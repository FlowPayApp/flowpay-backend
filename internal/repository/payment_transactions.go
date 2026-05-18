package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type PaymentTransactionRow struct {
	ID                 int64
	CompanyID          int64
	ClientID           int64
	PaymentTokenID     sql.NullInt64
	Gateway            string
	Environment        string
	BuyOrder           string
	SessionID          string
	WebpayToken        sql.NullString
	Amount             float64
	Currency           string
	Status             string
	AuthorizationCode  sql.NullString
	PaymentTypeCode    sql.NullString
	InstallmentsNumber sql.NullInt16
	CardLast4          sql.NullString
	ResponseCode       sql.NullInt32
	TransbankStatus    sql.NullString
	WebpayRedirectURL  sql.NullString
	ReturnURL          string
	RawCreate          json.RawMessage
	RawCommit          json.RawMessage
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CommittedAt        sql.NullTime
}

type PaymentTransactionChargeRow struct {
	TransactionID int64
	ChargeID      int64
	Amount        float64
}

func (r *Repository) InsertPaymentTransaction(ctx context.Context, row *PaymentTransactionRow) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
INSERT INTO payment_transactions (
  company_id, client_id, payment_token_id,
  gateway, environment, buy_order, session_id,
  amount, currency, status, return_url
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::payment_tx_status,$11)
RETURNING id
`, row.CompanyID, row.ClientID, nullInt64(row.PaymentTokenID),
		row.Gateway, row.Environment, row.BuyOrder, row.SessionID,
		row.Amount, row.Currency, row.Status, row.ReturnURL,
	).Scan(&id)
	return id, err
}

func (r *Repository) FailPaymentTransaction(ctx context.Context, id int64, note string, raw json.RawMessage) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE payment_transactions
SET status = 'failed'::payment_tx_status,
    transbank_status = NULLIF($2, ''),
    raw_create = COALESCE($3::jsonb, raw_create),
    updated_at = NOW()
WHERE id = $1
`, id, note, nullableJSON(raw))
	return err
}

func (r *Repository) UpdatePaymentTransactionWebpayCreate(ctx context.Context, id int64, webpayToken, webpayURL, status string, rawCreate json.RawMessage) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE payment_transactions
SET webpay_token = $2,
    status = $3::payment_tx_status,
    raw_create = $4::jsonb,
    webpay_redirect_url = NULLIF($5, ''),
    updated_at = NOW()
WHERE id = $1
`, id, webpayToken, status, nullableJSON(rawCreate), strings.TrimSpace(webpayURL))
	return err
}

func (r *Repository) UpdatePaymentTransactionAfterCommit(ctx context.Context, id int64, status string, resp *CommitUpdate) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE payment_transactions
SET status = $2::payment_tx_status,
    authorization_code = $3,
    payment_type_code = $4,
    installments_number = $5,
    card_last4 = $6,
    response_code = $7,
    transbank_status = $8,
    raw_commit = $9::jsonb,
    committed_at = NOW(),
    updated_at = NOW()
WHERE id = $1
`, id, status,
		nullStringVal(resp.AuthorizationCode), nullStringVal(resp.PaymentTypeCode),
		nullInt16Val(resp.InstallmentsNumber), nullStringVal(resp.CardLast4),
		nullInt32Val(resp.ResponseCode), nullStringVal(resp.TransbankStatus),
		nullableJSON(resp.RawCommit))
	return err
}

type CommitUpdate struct {
	AuthorizationCode  string
	PaymentTypeCode      string
	InstallmentsNumber int16
	CardLast4            string
	ResponseCode         int32
	TransbankStatus      string
	RawCommit            json.RawMessage
}

func (r *Repository) GetPaymentTransactionByID(ctx context.Context, id int64) (*PaymentTransactionRow, error) {
	return r.scanPaymentTransaction(r.db.QueryRowContext(ctx, paymentTransactionSelect+` WHERE id = $1`, id))
}

func (r *Repository) GetPaymentTransactionByWebpayToken(ctx context.Context, token string) (*PaymentTransactionRow, error) {
	return r.scanPaymentTransaction(r.db.QueryRowContext(ctx, paymentTransactionSelect+` WHERE webpay_token = $1`, token))
}

func (r *Repository) ListPaymentTransactionCharges(ctx context.Context, transactionID int64) ([]PaymentTransactionChargeRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT transaction_id, charge_id, amount
FROM payment_transaction_charges
WHERE transaction_id = $1
`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentTransactionChargeRow
	for rows.Next() {
		var row PaymentTransactionChargeRow
		if err := rows.Scan(&row.TransactionID, &row.ChargeID, &row.Amount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) InsertPaymentTransactionCharges(ctx context.Context, transactionID int64, items []PaymentTransactionChargeRow) error {
	for _, it := range items {
		if _, err := r.db.ExecContext(ctx, `
INSERT INTO payment_transaction_charges (transaction_id, charge_id, amount)
VALUES ($1, $2, $3)
`, transactionID, it.ChargeID, it.Amount); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) MarkChargePaidWebpay(ctx context.Context, chargeID int64, amount float64, transactionID int64, gatewayRef string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO payments (charge_id, amount, transaction_id, method, gateway_reference)
VALUES ($1, $2, $3, 'webpay', NULLIF($4, ''))
`, chargeID, amount, transactionID, gatewayRef); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE charges SET paid_at = CURRENT_TIMESTAMP WHERE id = $1 AND paid_at IS NULL`, chargeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) MarkPaymentTokenPaid(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE payment_tokens
SET status = 'paid'::payment_token_status
WHERE trim(both from token) = trim(both from $1::text)
  AND status IN ('issued'::payment_token_status, 'viewed'::payment_token_status)
`, token)
	return err
}

const paymentTransactionSelect = `
SELECT id, company_id, client_id, payment_token_id,
       gateway, environment, buy_order, session_id, webpay_token,
       amount, currency, status::text,
       authorization_code, payment_type_code, installments_number, card_last4,
       response_code, transbank_status, webpay_redirect_url, return_url,
       raw_create, raw_commit, created_at, updated_at, committed_at
FROM payment_transactions
`

func (r *Repository) scanPaymentTransaction(row *sql.Row) (*PaymentTransactionRow, error) {
	var out PaymentTransactionRow
	var rawCreateNS, rawCommitNS sql.NullString
	err := row.Scan(
		&out.ID, &out.CompanyID, &out.ClientID, &out.PaymentTokenID,
		&out.Gateway, &out.Environment, &out.BuyOrder, &out.SessionID, &out.WebpayToken,
		&out.Amount, &out.Currency, &out.Status,
		&out.AuthorizationCode, &out.PaymentTypeCode, &out.InstallmentsNumber, &out.CardLast4,
		&out.ResponseCode, &out.TransbankStatus, &out.WebpayRedirectURL, &out.ReturnURL,
		&rawCreateNS, &rawCommitNS, &out.CreatedAt, &out.UpdatedAt, &out.CommittedAt,
	)
	if err != nil {
		return nil, err
	}
	if rawCreateNS.Valid {
		out.RawCreate = json.RawMessage(rawCreateNS.String)
	}
	if rawCommitNS.Valid {
		out.RawCommit = json.RawMessage(rawCommitNS.String)
	}
	return &out, nil
}

func nullInt64(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}

func nullStringVal(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullInt16Val(n int16) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullInt32Val(n int32) any {
	return n
}

func nullInt16(v sql.NullInt16) any {
	if v.Valid {
		return v.Int16
	}
	return nil
}

func nullInt32(v sql.NullInt32) any {
	if v.Valid {
		return v.Int32
	}
	return nil
}

func nullableJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
