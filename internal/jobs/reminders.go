package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// StartReminderJob ejecuta un ciclo por intervalo para **todas** las empresas en `companies`.
func StartReminderJob(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, interval time.Duration, paymentsURL string) {
	ticker := time.NewTicker(interval)
	go func() {
		run := func() {
			ids, err := repo.ListCompanyIDs(context.Background())
			if err != nil {
				log.Println("[FlowPay Job] error listando empresas:", err)
				return
			}
			if len(ids) == 0 {
				log.Println("[FlowPay Job] sin empresas en BD; nada que procesar")
				return
			}
			for _, cid := range ids {
				runOnce(context.Background(), repo, d, cid, paymentsURL)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func runOnce(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, companyID int64, paymentsURL string) {
	log.Printf("[FlowPay Job] Inicio de ciclo de recordatorios (company_id=%d)…", companyID)
	dueSoon, err := repo.ChargesDueSoon(ctx, companyID, 5)
	if err != nil {
		log.Println("[FlowPay Job] error due_soon:", err)
		return
	}
	tn := truncate(time.Now())
	for _, ch := range dueSoon {
		if !allowAutoFollowUp(ch.ClientFollowupChannel) {
			continue
		}
		td := truncate(ch.DueDate)
		if tn.After(td) {
			continue
		}
		subject, _ := dueSoonTemplate(ch, tn, td)
		body, err := reminderBodyWithPayURL(ctx, repo, ch, "due_soon", paymentsURL)
		if err != nil {
			log.Println("[FlowPay Job] mensaje reminder_messages due_soon:", err)
			continue
		}
		dispatchChargeReminder(ctx, repo, d, ch, "due_soon", subject, body)
	}
	overdue, err := repo.ChargesOverdueUnpaid(ctx, companyID)
	if err != nil {
		log.Println("[FlowPay Job] error overdue:", err)
		return
	}
	for _, ch := range overdue {
		if !allowAutoFollowUp(ch.ClientFollowupChannel) {
			continue
		}
		priorOverdue, err := repo.CountRemindersByKind(ctx, ch.ID, "overdue")
		if err != nil {
			log.Println("[FlowPay Job] count overdue reminders:", err)
			priorOverdue = 0
		}
		subject, _ := overdueTemplate(ch, priorOverdue)
		body, err := reminderBodyWithPayURL(ctx, repo, ch, "overdue", paymentsURL)
		if err != nil {
			log.Println("[FlowPay Job] mensaje reminder_messages overdue:", err)
			continue
		}
		dispatchChargeReminder(ctx, repo, d, ch, "overdue", subject, body)
	}
	log.Printf("[FlowPay Job] Ciclo completado (company_id=%d).", companyID)
}

func reminderBodyWithPayURL(ctx context.Context, repo *repository.DB, ch repository.Charge, msgType, paymentsURL string) (string, error) {
	row, err := repo.GetActiveReminderMessage(ctx, ch.CompanyID, msgType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("sin mensaje activo en reminder_messages (company_id=%d)", ch.CompanyID)
		}
		return "", err
	}
	if row.Message == nil || strings.TrimSpace(*row.Message) == "" {
		return "", fmt.Errorf("mensaje vacío en reminder_messages (company_id=%d)", ch.CompanyID)
	}
	body := strings.TrimSpace(*row.Message)

	token, err := repo.LatestPaymentTokenForClient(ctx, ch.CompanyID, ch.ClientID)
	if err != nil {
		log.Printf("[FlowPay Job] payment_tokens client_id=%d: %v", ch.ClientID, err)
	}
	if payURL := buildPayPageURL(paymentsURL, token); payURL != "" {
		body = strings.TrimRight(body, "\n") + "\n\n" + payURL
	} else {
		log.Printf("[FlowPay Job] sin URL de pago (company_id=%d client_id=%d)", ch.CompanyID, ch.ClientID)
	}
	return body, nil
}

func buildPayPageURL(base, token string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	token = strings.TrimSpace(token)
	if base == "" || token == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(base), "/pay") {
		return base + "/" + token
	}
	return base + "/pay/" + token
}

func dispatchChargeReminder(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, ch repository.Charge, kind, subject, body string) {
	if ok, _ := shouldPersist(ctx, repo, ch.ID, kind); !ok {
		return
	}
	if shouldSendEmail(ch.ClientFollowupChannel) {
		log.Printf("[FlowPay Job] email cobro=#%d kind=%s\nAsunto: %s\n%s", ch.ID, kind, subject, body)
		if d != nil {
			d.SendReminderEmail(ch, subject, body)
		}
		emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subject, body)
		if _, err := repo.InsertReminder(ctx, ch.ID, kind, "email", "sent", emailMessage, ptrNow()); err != nil {
			log.Println("[FlowPay Job] insert reminder:", err)
		}
	}
	if shouldSendWhatsApp(ch.ClientFollowupChannel) {
		log.Printf("[FlowPay Job] whatsapp cobro=#%d kind=%s\n%s", ch.ID, kind, body)
		if d != nil {
			d.SendReminderWhatsApp(ch, body)
		}
		if _, err := repo.InsertReminder(ctx, ch.ID, kind, "whatsapp", "sent", body, ptrNow()); err != nil {
			log.Println("[FlowPay Job] insert reminder WA:", err)
		}
	}
}

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func ptrNow() *time.Time {
	t := time.Now()
	return &t
}

func dueSoonTemplate(ch repository.Charge, today, dueDate time.Time) (subject string, body string) {
	if today.Before(dueDate) {
		return "Recordatorio: cobro próximo a vencer", notify.BodyApproaching(ch)
	}
	return "Hoy vence un cobro pendiente", notify.BodyDueToday(ch)
}

func overdueTemplate(ch repository.Charge, priorOverdueReminders int) (subject string, body string) {
	if priorOverdueReminders == 0 {
		return "Cobro vencido — acción requerida", notify.BodyOverdueFirst(ch)
	}
	return "Seguimiento de cobro pendiente", notify.BodyOverdueFollowUp(ch)
}

func shouldPersist(ctx context.Context, repo *repository.DB, chargeID int64, kind string) (bool, error) {
	n, err := repo.CountRecentReminders(ctx, chargeID, kind, 20*time.Hour)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

func normalizeChannel(ch string) string {
	v := strings.TrimSpace(strings.ToLower(ch))
	if v == "" {
		return "all"
	}
	return v
}

func allowAutoFollowUp(ch string) bool {
	return normalizeChannel(ch) != "none"
}

func shouldSendEmail(ch string) bool {
	v := normalizeChannel(ch)
	return v == "all" || v == "email"
}

func shouldSendWhatsApp(ch string) bool {
	v := normalizeChannel(ch)
	return v == "all" || v == "whatsapp"
}
