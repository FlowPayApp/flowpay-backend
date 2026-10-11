package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/flowpay/flowpay-backend/internal/domain"
)

// ChargeReminderSettings frecuencia y canal de los recordatorios automáticos de un cobro.
type ChargeReminderSettings struct {
	Mode string
	// Channel vacío usa el canal preferido de la sucursal.
	Channel string
	// Policy solo rige con Mode custom.
	Policy domain.ReminderPolicy
}

// AutoReminderCharge cobro sin pagar con lo que necesita el ciclo diario de recordatorios.
type AutoReminderCharge struct {
	Charge
	Reminders ChargeReminderSettings
}

func (db *DB) GetCompanyReminderPolicy(ctx context.Context, companyID int64) (domain.ReminderPolicy, error) {
	var days string
	var p domain.ReminderPolicy
	err := db.db.QueryRowContext(ctx, `
SELECT reminder_days_before, reminder_overdue_every, reminder_overdue_max FROM companies WHERE id = $1`,
		companyID).Scan(&days, &p.OverdueEvery, &p.OverdueMax)
	if err != nil {
		return domain.ReminderPolicy{}, err
	}
	p.DaysBefore = domain.ParseReminderDays(days)
	return p, nil
}

func (db *DB) UpdateCompanyReminderPolicy(ctx context.Context, companyID int64, p domain.ReminderPolicy) error {
	_, err := db.db.ExecContext(ctx, `
UPDATE companies SET reminder_days_before = $1, reminder_overdue_every = $2, reminder_overdue_max = $3 WHERE id = $4`,
		domain.FormatReminderDays(p.DaysBefore), p.OverdueEvery, p.OverdueMax, companyID)
	return err
}

func (db *DB) GetChargeReminderSettings(ctx context.Context, chargeID int64) (ChargeReminderSettings, error) {
	var s ChargeReminderSettings
	var days string
	err := db.db.QueryRowContext(ctx, `
SELECT reminder_mode, reminder_channel, reminder_days_before, reminder_overdue_every, reminder_overdue_max
FROM charges WHERE id = $1`, chargeID).Scan(&s.Mode, &s.Channel, &days, &s.Policy.OverdueEvery, &s.Policy.OverdueMax)
	if err != nil {
		return s, err
	}
	s.Policy.DaysBefore = domain.ParseReminderDays(days)
	return s, nil
}

func (db *DB) UpdateChargeReminderSettings(ctx context.Context, companyID, chargeID int64, s ChargeReminderSettings) error {
	res, err := db.db.ExecContext(ctx, `
UPDATE charges SET reminder_mode = $1, reminder_channel = $2, reminder_days_before = $3,
       reminder_overdue_every = $4, reminder_overdue_max = $5
WHERE id = $6 AND company_id = $7`,
		s.Mode, s.Channel, domain.FormatReminderDays(s.Policy.DaysBefore), s.Policy.OverdueEvery, s.Policy.OverdueMax,
		chargeID, companyID)
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

// ChargesForAutoReminders cobros sin pagar de la empresa que no tienen los recordatorios automáticos apagados.
func (db *DB) ChargesForAutoReminders(ctx context.Context, companyID int64) ([]AutoReminderCharge, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.attachment_ext, i.created_at, `+chargeClientLabelExpr+`, c.email, c.phone, c.followup_channel,
       i.reminder_mode, i.reminder_channel, i.reminder_days_before, i.reminder_overdue_every, i.reminder_overdue_max
FROM charges i
JOIN clients c ON c.id = i.client_id
WHERE i.company_id = $1
  AND i.paid_at IS NULL
  AND i.reminder_mode <> 'off'
ORDER BY i.due_date`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutoReminderCharge
	for rows.Next() {
		var item AutoReminderCharge
		ch := &item.Charge
		var ce, cp, atok, aext sql.NullString
		var days string
		if err := rows.Scan(&ch.ID, &ch.CompanyID, &ch.ClientID, &ch.Amount, &ch.DueDate, &ch.PaidAt,
			&atok, &aext, &ch.CreatedAt, &ch.ClientName, &ce, &cp, &ch.ClientFollowupChannel,
			&item.Reminders.Mode, &item.Reminders.Channel, &days,
			&item.Reminders.Policy.OverdueEvery, &item.Reminders.Policy.OverdueMax); err != nil {
			return nil, err
		}
		ch.ClientEmail = assignNullString(ce)
		ch.ClientPhone = assignNullString(cp)
		ch.AttachmentToken = assignNullString(atok)
		ch.AttachmentExt = assignNullString(aext)
		item.Reminders.Policy.DaysBefore = domain.ParseReminderDays(days)
		out = append(out, item)
	}
	return out, rows.Err()
}

// OverdueReminderHistory días distintos en que se avisó del cobro vencido y el último aviso.
func (db *DB) OverdueReminderHistory(ctx context.Context, chargeID int64) (sends int, last *time.Time, err error) {
	err = db.db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT created_at::date), MAX(created_at)
FROM reminders WHERE charge_id = $1 AND kind = 'overdue'`, chargeID).Scan(&sends, &last)
	return sends, last, err
}
