package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func assignNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

type Client struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"company_id"`
	Name      string    `json:"name"`
	Email     *string   `json:"email,omitempty"`
	Phone     *string   `json:"phone,omitempty"`
	IsActive  bool      `json:"is_active"`
	FollowupChannel string `json:"followup_channel"`
	CreatedAt time.Time `json:"created_at"`
}

// Charge = cobro / monto que te deben (no es DTE ni factura electrónica).
type Charge struct {
	ID          int64      `json:"id"`
	CompanyID   int64      `json:"company_id"`
	ClientID    int64      `json:"client_id"`
	Amount      float64    `json:"amount"`
	DueDate     time.Time  `json:"due_date"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ClientName  string     `json:"client_name,omitempty"`
	ClientEmail *string    `json:"client_email,omitempty"`
	ClientPhone *string    `json:"client_phone,omitempty"`
	ClientFollowupChannel string `json:"client_followup_channel,omitempty"`
	// Adjunto (PDF/imagen) para enviar por WhatsApp; token sirve para URL pública.
	AttachmentToken *string `json:"attachment_token,omitempty"`
	AttachmentExt   *string `json:"attachment_ext,omitempty"`
}

type Payment struct {
	ID        int64     `json:"id"`
	ChargeID  int64     `json:"charge_id"`
	Amount    float64   `json:"amount"`
	PaidAt    time.Time `json:"paid_at"`
	CreatedAt time.Time `json:"created_at"`
}

type Reminder struct {
	ID        int64      `json:"id"`
	ChargeID  int64      `json:"charge_id"`
	Kind      string     `json:"kind"`
	Channel   string     `json:"channel"`
	Status    string     `json:"status"`
	Message   *string    `json:"message,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
}

type DashboardTotals struct {
	PendingAmount float64 `json:"pending_amount"`
	OverdueAmount float64 `json:"overdue_amount"`
	PaidAmount    float64 `json:"paid_amount"`
	PendingCount  int     `json:"pending_count"`
	OverdueCount  int     `json:"overdue_count"`
	PaidCount     int     `json:"paid_count"`
}

type CompanyOverviewRow struct {
	CompanyID     int64   `json:"company_id"`
	CompanyName   string  `json:"company_name"`
	PaidAmount    float64 `json:"paid_amount"`
	PendingAmount float64 `json:"pending_amount"`
	OverdueAmount float64 `json:"overdue_amount"`
	OwedAmount    float64 `json:"owed_amount"`
}

type ClientRow struct {
	Client
	TotalOwed  float64 `json:"total_owed"`
	OverdueCnt int     `json:"overdue_count"`
}

type Repository struct {
	db *sql.DB
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ListCompanyIDs devuelve todos los negocios (tenants) registrados — p. ej. para jobs multi-empresa.
func (r *Repository) ListCompanyIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM companies ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) ListClients(ctx context.Context, companyID int64) ([]ClientRow, error) {
	q := `
SELECT c.id, c.company_id, c.name, c.email, c.phone, c.created_at,
       c.is_active,
       c.followup_channel,
       COALESCE(SUM(CASE WHEN i.paid_at IS NULL THEN i.amount ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN i.paid_at IS NULL AND i.due_date < CURDATE() THEN 1 ELSE 0 END), 0)
FROM clients c
LEFT JOIN charges i ON i.client_id = c.id AND i.company_id = c.company_id
WHERE c.company_id = ?
GROUP BY c.id, c.company_id, c.name, c.email, c.phone, c.created_at, c.is_active, c.followup_channel
ORDER BY c.created_at DESC, c.id DESC
`
	rows, err := r.db.QueryContext(ctx, q, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientRow
	for rows.Next() {
		var cr ClientRow
		var isActive int
		if err := rows.Scan(&cr.ID, &cr.CompanyID, &cr.Name, &cr.Email, &cr.Phone, &cr.CreatedAt, &isActive, &cr.FollowupChannel, &cr.TotalOwed, &cr.OverdueCnt); err != nil {
			return nil, err
		}
		cr.IsActive = isActive != 0
		if cr.FollowupChannel == "" {
			cr.FollowupChannel = "all"
		}
		out = append(out, cr)
	}
	return out, rows.Err()
}

func (r *Repository) CreateClient(ctx context.Context, companyID int64, name, email, phone string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO clients (company_id, name, email, phone, is_active, followup_channel) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), 1, 'all')`,
		companyID, name, email, phone,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) SetClientActive(ctx context.Context, companyID, clientID int64, isActive bool) error {
	active := 0
	if isActive {
		active = 1
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE clients SET is_active = ? WHERE id = ? AND company_id = ?`,
		active, clientID, companyID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) PatchClient(ctx context.Context, companyID, clientID int64, isActive *bool, followupChannel *string) error {
	var sets []string
	var args []any
	if isActive != nil {
		iv := 0
		if *isActive {
			iv = 1
		}
		sets = append(sets, "is_active = ?")
		args = append(args, iv)
	}
	if followupChannel != nil {
		sets = append(sets, "followup_channel = ?")
		args = append(args, strings.TrimSpace(strings.ToLower(*followupChannel)))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, clientID, companyID)
	q := fmt.Sprintf("UPDATE clients SET %s WHERE id = ? AND company_id = ?", strings.Join(sets, ", "))
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanChargeClient(rows *sql.Rows, ch *Charge) error {
	var ce, cp, atok, aext sql.NullString
	if err := rows.Scan(&ch.ID, &ch.CompanyID, &ch.ClientID, &ch.Amount, &ch.DueDate, &ch.PaidAt,
		&atok, &aext, &ch.CreatedAt, &ch.ClientName, &ce, &cp, &ch.ClientFollowupChannel); err != nil {
		return err
	}
	ch.ClientEmail = assignNullString(ce)
	ch.ClientPhone = assignNullString(cp)
	ch.AttachmentToken = assignNullString(atok)
	ch.AttachmentExt = assignNullString(aext)
	if ch.ClientFollowupChannel == "" {
		ch.ClientFollowupChannel = "all"
	}
	return nil
}

func (r *Repository) ListCharges(ctx context.Context, companyID int64) ([]Charge, error) {
	q := `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.attachment_ext, i.created_at, c.name, c.email, c.phone, c.followup_channel
FROM charges i
JOIN clients c ON c.id = i.client_id
WHERE i.company_id = ?
ORDER BY i.due_date ASC, i.id ASC
`
	rows, err := r.db.QueryContext(ctx, q, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Charge
	for rows.Next() {
		var ch Charge
		if err := scanChargeClient(rows, &ch); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (r *Repository) GetCharge(ctx context.Context, companyID, id int64) (*Charge, error) {
	q := `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.attachment_ext, i.created_at, c.name, c.email, c.phone, c.followup_channel
FROM charges i
JOIN clients c ON c.id = i.client_id
WHERE i.company_id = ? AND i.id = ?
`
	var ch Charge
	var ce, cp, atok, aext sql.NullString
	err := r.db.QueryRowContext(ctx, q, companyID, id).Scan(
		&ch.ID, &ch.CompanyID, &ch.ClientID, &ch.Amount, &ch.DueDate, &ch.PaidAt,
		&atok, &aext, &ch.CreatedAt, &ch.ClientName, &ce, &cp, &ch.ClientFollowupChannel,
	)
	if err != nil {
		return nil, err
	}
	ch.ClientEmail = assignNullString(ce)
	ch.ClientPhone = assignNullString(cp)
	ch.AttachmentToken = assignNullString(atok)
	ch.AttachmentExt = assignNullString(aext)
	return &ch, nil
}

func (r *Repository) CreateCharge(ctx context.Context, companyID, clientID int64, amount float64, dueDate time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO charges (company_id, client_id, amount, due_date) VALUES (?, ?, ?, ?)`,
		companyID, clientID, amount, dueDate.Format("2006-01-02"),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) MarkChargePaid(ctx context.Context, chargeID int64, amount float64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO payments (charge_id, amount) VALUES (?, ?)`, chargeID, amount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE charges SET paid_at = CURRENT_TIMESTAMP WHERE id = ?`, chargeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ListReminders(ctx context.Context, chargeID int64) ([]Reminder, error) {
	q := `
SELECT id, charge_id, kind, channel, status, message, created_at, sent_at
FROM reminders WHERE charge_id = ? ORDER BY created_at ASC, id ASC
`
	rows, err := r.db.QueryContext(ctx, q, chargeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reminder
	for rows.Next() {
		var rm Reminder
		if err := rows.Scan(&rm.ID, &rm.ChargeID, &rm.Kind, &rm.Channel, &rm.Status, &rm.Message, &rm.CreatedAt, &rm.SentAt); err != nil {
			return nil, err
		}
		out = append(out, rm)
	}
	return out, rows.Err()
}

func (r *Repository) InsertReminder(ctx context.Context, chargeID int64, kind, channel, status, message string, sentAt *time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO reminders (charge_id, kind, channel, status, message, sent_at) VALUES (?, ?, ?, ?, ?, ?)`,
		chargeID, kind, channel, status, message, sentAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ChargesDueSoon: no cobrados, due_date entre hoy y hoy+days (inclusive upper bound por día).
func (r *Repository) ChargesDueSoon(ctx context.Context, companyID int64, days int) ([]Charge, error) {
	q := `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.attachment_ext, i.created_at, c.name, c.email, c.phone, c.followup_channel
FROM charges i
JOIN clients c ON c.id = i.client_id
WHERE i.company_id = ?
  AND i.paid_at IS NULL
  AND i.due_date >= CURDATE()
  AND i.due_date <= DATE_ADD(CURDATE(), INTERVAL ? DAY)
ORDER BY i.due_date
`
	return r.queryCharges(ctx, q, companyID, days)
}

func (r *Repository) ChargesOverdueUnpaid(ctx context.Context, companyID int64) ([]Charge, error) {
	q := `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.attachment_ext, i.created_at, c.name, c.email, c.phone, c.followup_channel
FROM charges i
JOIN clients c ON c.id = i.client_id
WHERE i.company_id = ?
  AND i.paid_at IS NULL
  AND i.due_date < CURDATE()
ORDER BY i.due_date
`
	return r.queryCharges(ctx, q, companyID)
}

func (r *Repository) queryCharges(ctx context.Context, query string, args ...any) ([]Charge, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Charge
	for rows.Next() {
		var ch Charge
		if err := scanChargeClient(rows, &ch); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (r *Repository) CountRecentReminders(ctx context.Context, chargeID int64, kind string, within time.Duration) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reminders WHERE charge_id = ? AND kind = ? AND created_at > DATE_SUB(NOW(), INTERVAL ? SECOND)`,
		chargeID, kind, int(within.Seconds()),
	).Scan(&n)
	return n, err
}

// CountRemindersByKind cuenta todos los recordatorios de un tipo (p. ej. overdue acumulados).
func (r *Repository) CountRemindersByKind(ctx context.Context, chargeID int64, kind string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reminders WHERE charge_id = ? AND kind = ?`,
		chargeID, kind,
	).Scan(&n)
	return n, err
}

// SetChargeAttachment guarda token y extensión del archivo (sin punto).
func (r *Repository) SetChargeAttachment(ctx context.Context, companyID, chargeID int64, token, ext string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE charges SET attachment_token = ?, attachment_ext = ? WHERE id = ? AND company_id = ?`,
		token, ext, chargeID, companyID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetAttachmentExtByToken devuelve la extensión si existe cobro con ese token.
func (r *Repository) GetAttachmentExtByToken(ctx context.Context, token string) (string, error) {
	var ext sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT attachment_ext FROM charges WHERE attachment_token = ? AND attachment_token IS NOT NULL`,
		token,
	).Scan(&ext)
	if err != nil {
		return "", err
	}
	if !ext.Valid || ext.String == "" {
		return "", errors.New("empty attachment")
	}
	return ext.String, nil
}

// ClientBelongsToCompany comprueba que el cliente pertenece a la empresa.
func (r *Repository) ClientBelongsToCompany(ctx context.Context, companyID, clientID int64) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM clients WHERE id = ? AND company_id = ? LIMIT 1`,
		clientID, companyID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) ActiveClientBelongsToCompany(ctx context.Context, companyID, clientID int64) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM clients WHERE id = ? AND company_id = ? AND is_active = 1 LIMIT 1`,
		clientID, companyID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpdateChargeFields actualiza solo los campos no nil.
func (r *Repository) UpdateChargeFields(ctx context.Context, companyID, chargeID int64, clientID *int64, dueDate *time.Time, amount *float64) error {
	var sets []string
	var args []any
	if clientID != nil {
		sets = append(sets, "client_id = ?")
		args = append(args, *clientID)
	}
	if dueDate != nil {
		sets = append(sets, "due_date = ?")
		args = append(args, dueDate.Format("2006-01-02"))
	}
	if amount != nil {
		sets = append(sets, "amount = ?")
		args = append(args, *amount)
	}
	if len(sets) == 0 {
		return nil
	}
	var one int
	if err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM charges WHERE id = ? AND company_id = ? LIMIT 1`,
		chargeID, companyID,
	).Scan(&one); err != nil {
		return err
	}
	args = append(args, chargeID, companyID)
	q := fmt.Sprintf("UPDATE charges SET %s WHERE id = ? AND company_id = ?", strings.Join(sets, ", "))
	_, err := r.db.ExecContext(ctx, q, args...)
	return err
}

// ClearChargePayment quita el estado cobrado y los pagos asociados (MVP).
func (r *Repository) ClearChargePayment(ctx context.Context, companyID, chargeID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM payments WHERE charge_id = ?`, chargeID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE charges SET paid_at = NULL WHERE id = ? AND company_id = ?`, chargeID, companyID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (r *Repository) DashboardAggregate(ctx context.Context, companyID int64) (DashboardTotals, error) {
	var t DashboardTotals
	q := `
SELECT
  COALESCE(SUM(CASE WHEN paid_at IS NOT NULL THEN amount ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN paid_at IS NOT NULL THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN paid_at IS NULL AND due_date >= CURDATE() THEN amount ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN paid_at IS NULL AND due_date >= CURDATE() THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN paid_at IS NULL AND due_date < CURDATE() THEN amount ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN paid_at IS NULL AND due_date < CURDATE() THEN 1 ELSE 0 END), 0)
FROM charges
WHERE company_id = ?
`
	err := r.db.QueryRowContext(ctx, q, companyID).Scan(
		&t.PaidAmount, &t.PaidCount,
		&t.PendingAmount, &t.PendingCount,
		&t.OverdueAmount, &t.OverdueCount,
	)
	return t, err
}

func (r *Repository) PlatformCompaniesOverview(ctx context.Context) ([]CompanyOverviewRow, error) {
	q := `
SELECT
  c.id,
  c.name,
  (
    SELECT COALESCE(SUM(ch.amount), 0)
    FROM charges ch
    WHERE ch.company_id = c.id
      AND ch.paid_at IS NOT NULL
  ) AS paid_amount,
  (
    SELECT COALESCE(SUM(ch.amount), 0)
    FROM charges ch
    WHERE ch.company_id = c.id
      AND ch.paid_at IS NULL
      AND ch.due_date >= CURDATE()
  ) AS pending_amount,
  (
    SELECT COALESCE(SUM(ch.amount), 0)
    FROM charges ch
    WHERE ch.company_id = c.id
      AND ch.paid_at IS NULL
      AND ch.due_date < CURDATE()
  ) AS overdue_amount
FROM companies c
ORDER BY c.id ASC
`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CompanyOverviewRow
	for rows.Next() {
		var row CompanyOverviewRow
		if err := rows.Scan(
			&row.CompanyID,
			&row.CompanyName,
			&row.PaidAmount,
			&row.PendingAmount,
			&row.OverdueAmount,
		); err != nil {
			return nil, err
		}
		row.OwedAmount = row.PendingAmount + row.OverdueAmount
		out = append(out, row)
	}
	return out, rows.Err()
}
