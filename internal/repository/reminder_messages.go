package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ReminderMessage plantilla/mensaje de recordatorio por empresa.
type ReminderMessage struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	CompanyID int64     `json:"company_id"`
	IsActive  *bool     `json:"is_active"`
	Message   *string   `json:"message"`
	Type      *string   `json:"type"`
}

func scanReminderMessage(id int64, createdAt time.Time, companyID int64, isActive sql.NullBool, message, msgType sql.NullString) ReminderMessage {
	row := ReminderMessage{
		ID:        id,
		CreatedAt: createdAt,
		CompanyID: companyID,
	}
	if isActive.Valid {
		v := isActive.Bool
		row.IsActive = &v
	}
	row.Message = assignNullString(message)
	row.Type = assignNullString(msgType)
	return row
}

func (db *DB) CreateReminderMessage(ctx context.Context, companyID int64, isActive *bool, message, msgType string) (*ReminderMessage, error) {
	var (
		id        int64
		createdAt time.Time
		coID      int64
		active    sql.NullBool
		msg       sql.NullString
		typ       sql.NullString
	)
	err := db.db.QueryRowContext(ctx, `
INSERT INTO reminder_messages (company_id, is_active, message, "type")
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, company_id, is_active, message, "type"
`, companyID, isActive, nullStr(message), nullStr(msgType)).Scan(&id, &createdAt, &coID, &active, &msg, &typ)
	if err != nil {
		return nil, err
	}
	row := scanReminderMessage(id, createdAt, coID, active, msg, typ)
	return &row, nil
}

// GetActiveReminderMessage devuelve el mensaje activo de la empresa.
// Si msgType no está vacío, prioriza filas cuyo type coincide (p. ej. due_soon / overdue).
func (db *DB) GetActiveReminderMessage(ctx context.Context, companyID int64, msgType string) (*ReminderMessage, error) {
	msgType = strings.ToLower(strings.TrimSpace(msgType))
	var (
		id        int64
		createdAt time.Time
		coID      int64
		active    sql.NullBool
		msg       sql.NullString
		typ       sql.NullString
	)
	err := db.db.QueryRowContext(ctx, `
SELECT id, created_at, company_id, is_active, message, "type"
FROM reminder_messages
WHERE company_id = $1
  AND COALESCE(is_active, TRUE) = TRUE
  AND NULLIF(TRIM(COALESCE(message, '')), '') IS NOT NULL
ORDER BY
  CASE WHEN $2 <> '' AND lower(trim(coalesce("type", ''))) = $2 THEN 0 ELSE 1 END,
  id DESC
LIMIT 1
`, companyID, msgType).Scan(&id, &createdAt, &coID, &active, &msg, &typ)
	if err != nil {
		return nil, err
	}
	row := scanReminderMessage(id, createdAt, coID, active, msg, typ)
	return &row, nil
}

func (db *DB) GetReminderMessage(ctx context.Context, id int64) (*ReminderMessage, error) {
	var (
		createdAt time.Time
		coID      int64
		active    sql.NullBool
		msg       sql.NullString
		typ       sql.NullString
	)
	err := db.db.QueryRowContext(ctx, `
SELECT id, created_at, company_id, is_active, message, "type"
FROM reminder_messages
WHERE id = $1
`, id).Scan(&id, &createdAt, &coID, &active, &msg, &typ)
	if err != nil {
		return nil, err
	}
	row := scanReminderMessage(id, createdAt, coID, active, msg, typ)
	return &row, nil
}

func (db *DB) UpdateReminderMessage(ctx context.Context, id int64, companyID *int64, message *string, isActive *bool) error {
	var sets []string
	var args []any
	n := 1
	if message != nil {
		sets = append(sets, fmt.Sprintf("message = $%d", n))
		args = append(args, nullStr(*message))
		n++
	}
	if isActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", n))
		args = append(args, *isActive)
		n++
	}
	if len(sets) == 0 {
		return nil
	}

	where := fmt.Sprintf("id = $%d", n)
	args = append(args, id)
	n++
	if companyID != nil {
		where += fmt.Sprintf(" AND company_id = $%d", n)
		args = append(args, *companyID)
	}

	q := fmt.Sprintf("UPDATE reminder_messages SET %s WHERE %s", strings.Join(sets, ", "), where)
	res, err := db.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
