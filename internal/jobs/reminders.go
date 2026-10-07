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
	"github.com/flowpay/flowpay-backend/internal/remindercontent"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// StartReminderJob ejecuta un ciclo por intervalo para **todas** las empresas en `companies`.
func StartReminderJob(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, interval time.Duration, appPublicURL string) {
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
				runOnce(context.Background(), repo, d, cid, appPublicURL)
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

func runOnce(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, companyID int64, appPublicURL string) {
	log.Printf("[FlowPay Job] Inicio de ciclo de recordatorios (company_id=%d)…", companyID)
	mailbox := companyMailbox(ctx, repo, companyID)
	waFrom, waErr := repo.FirstActiveWhatsAppToForCompany(ctx, companyID)
	if waErr != nil && !errors.Is(waErr, sql.ErrNoRows) {
		log.Println("[FlowPay Job] número WhatsApp:", waErr)
	}
	if waErr != nil {
		waFrom = ""
	}
	dueSoon, err := repo.ChargesDueSoon(ctx, companyID, 5, 0)
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
		var phase string
		var daysUntil int
		switch {
		case tn.Before(td):
			phase = remindercontent.PhaseApproaching
			daysUntil = remindercontent.CalendarDaysUntilDue(tn, td)
		case tn.Equal(td):
			phase = remindercontent.PhaseDueToday
			daysUntil = 0
		default:
			continue
		}
		subject, body, _, payURL, err := remindercontent.ResolveReminder(ctx, repo, ch.CompanyID, phase, daysUntil, 0, ch, appPublicURL)
		if err != nil {
			log.Println("[FlowPay Job] resolve template due_soon:", err)
			subject, body = dueSoonTemplate(ch, tn, td)
		}
		log.Println("[FlowPay Job]", subject)
		if ok, _ := shouldPersist(ctx, repo, ch.ID, "due_soon"); ok {
			if shouldSendEmail(ch.ClientFollowupChannel) {
				if sendCompanyEmail(d, ch, subject, body, mailbox) {
					emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subject, body)
					if _, err := repo.InsertReminder(ctx, ch.ID, "due_soon", "email", "sent", emailMessage, ptrNow()); err != nil {
						log.Println("[FlowPay Job] insert reminder:", err)
					}
				}
			}
			if shouldSendWhatsApp(ch.ClientFollowupChannel) {
				if preview, ok := sendCompanyWhatsAppTemplate(d, ch, waFrom, phase, payURL); ok {
					if _, err := repo.InsertReminder(ctx, ch.ID, "due_soon", "whatsapp", "sent", preview, ptrNow()); err != nil {
						log.Println("[FlowPay Job] insert reminder WA:", err)
					}
				}
			}
		}
	}
	overdue, err := repo.ChargesOverdueUnpaid(ctx, companyID, 0)
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
		phase := remindercontent.PhaseOverdueFollowUp
		if priorOverdue == 0 {
			phase = remindercontent.PhaseOverdueFirst
		}
		subject, body, _, payURL, err := remindercontent.ResolveReminder(ctx, repo, ch.CompanyID, phase, 0, priorOverdue, ch, appPublicURL)
		if err != nil {
			log.Println("[FlowPay Job] resolve template overdue:", err)
			subject, body = overdueTemplate(ch, priorOverdue)
		}
		log.Println("[FlowPay Job]", subject)
		if ok, _ := shouldPersist(ctx, repo, ch.ID, "overdue"); ok {
			if shouldSendEmail(ch.ClientFollowupChannel) {
				if sendCompanyEmail(d, ch, subject, body, mailbox) {
					emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subject, body)
					if _, err := repo.InsertReminder(ctx, ch.ID, "overdue", "email", "sent", emailMessage, ptrNow()); err != nil {
						log.Println("[FlowPay Job] insert reminder:", err)
					}
				}
			}
			if shouldSendWhatsApp(ch.ClientFollowupChannel) {
				if preview, ok := sendCompanyWhatsAppTemplate(d, ch, waFrom, phase, payURL); ok {
					if _, err := repo.InsertReminder(ctx, ch.ID, "overdue", "whatsapp", "sent", preview, ptrNow()); err != nil {
						log.Println("[FlowPay Job] insert reminder WA:", err)
					}
				}
			}
		}
	}
	log.Printf("[FlowPay Job] Ciclo completado (company_id=%d).", companyID)
}

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func ptrNow() *time.Time {
	t := time.Now()
	return &t
}

func daysBetween(a, b time.Time) int {
	d := int(b.Sub(a).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
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

func companyMailbox(ctx context.Context, repo *repository.DB, companyID int64) *notify.SMTPConfig {
	m, err := repo.GetCompanyMailbox(ctx, companyID)
	if err != nil || m == nil {
		return nil
	}
	cfg := &notify.SMTPConfig{
		Host:     m.SMTPHost,
		Port:     m.SMTPPort,
		Username: m.SMTPUsername,
		Password: m.SMTPPassword,
		From:     m.FromEmail,
		FromName: m.FromName,
	}
	if !cfg.Enabled() {
		return nil
	}
	return cfg
}

func sendCompanyEmail(d *notify.Dispatcher, ch repository.Charge, subject, body string, mailbox *notify.SMTPConfig) bool {
	if d == nil {
		return true
	}
	if err := d.SendReminderEmailFrom(ch, subject, body, mailbox); err != nil {
		log.Println("[FlowPay Job] correo:", err)
		return false
	}
	return true
}

func sendCompanyWhatsAppTemplate(d *notify.Dispatcher, ch repository.Charge, from, phase, payURL string) (string, bool) {
	msg, ok := notify.BuildWhatsAppTemplate(phase, ch, payURL, notify.TemplateSIDs{})
	if !ok {
		log.Println("[FlowPay Job] WhatsApp: fase sin plantilla", phase)
		return "", false
	}
	if d == nil {
		return msg.Preview, true
	}
	preview, err := d.SendCompanyWhatsAppTemplate(ch, from, phase, payURL)
	if err != nil {
		log.Println("[FlowPay Job] WhatsApp:", err)
		return "", false
	}
	if preview == "" {
		preview = msg.Preview
	}
	return preview, true
}
