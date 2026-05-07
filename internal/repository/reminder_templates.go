package repository

import (
	"context"
	"strings"
)

// ReminderTemplateRow plantilla persistida.
type ReminderTemplateRow struct {
	ID            int64  `json:"id"`
	CompanyID     int64  `json:"company_id"`
	Phase         string `json:"phase"`
	DayMin        int    `json:"day_min"`
	DayMax        int    `json:"day_max"`
	SortOrder     int    `json:"sort_order"`
	EmailSubject  string `json:"email_subject"`
	Body          string `json:"body"`
}

func (r *Repository) ListReminderTemplates(ctx context.Context, companyID int64) ([]ReminderTemplateRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, company_id, phase, day_min, day_max, sort_order, COALESCE(email_subject,''), body
FROM company_reminder_templates
WHERE company_id = $1
ORDER BY phase ASC, sort_order ASC, day_min ASC, id ASC
`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReminderTemplateRow
	for rows.Next() {
		var t ReminderTemplateRow
		if err := rows.Scan(&t.ID, &t.CompanyID, &t.Phase, &t.DayMin, &t.DayMax, &t.SortOrder, &t.EmailSubject, &t.Body); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReplaceReminderTemplates sustituye todas las plantillas del tenant (transacción).
func (r *Repository) ReplaceReminderTemplates(ctx context.Context, companyID int64, list []ReminderTemplateRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM company_reminder_templates WHERE company_id = $1`, companyID); err != nil {
		return err
	}
	for _, t := range list {
		ph := strings.TrimSpace(strings.ToLower(t.Phase))
		if ph == "" || strings.TrimSpace(t.Body) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO company_reminder_templates (company_id, phase, day_min, day_max, sort_order, email_subject, body)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, companyID, ph, t.DayMin, t.DayMax, t.SortOrder, strings.TrimSpace(t.EmailSubject), t.Body); err != nil {
			return err
		}
	}
	return tx.Commit()
}
