package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// StartReminderJob ejecuta un ciclo por intervalo para **todas** las empresas en `companies`.
func StartReminderJob(ctx context.Context, repo *repository.Repository, d *notify.Dispatcher, interval time.Duration) {
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
				runOnce(context.Background(), repo, d, cid)
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

func runOnce(ctx context.Context, repo *repository.Repository, d *notify.Dispatcher, companyID int64) {
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
		subject, body := dueSoonTemplate(ch, tn, td)
		log.Println("[FlowPay Job]", subject)
		if ok, _ := shouldPersist(ctx, repo, ch.ID, "due_soon"); ok {
			if shouldSendEmail(ch.ClientFollowupChannel) {
				if d != nil {
					if tn.Before(td) {
						d.SendApproachingEmail(ch)
					} else if tn.Equal(td) {
						d.SendDueTodayEmail(ch)
					}
				}
				emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subject, body)
				if _, err := repo.InsertReminder(ctx, ch.ID, "due_soon", "email", "sent", emailMessage, ptrNow()); err != nil {
					log.Println("[FlowPay Job] insert reminder:", err)
				}
			}
			if shouldSendWhatsApp(ch.ClientFollowupChannel) {
				if d != nil {
					if tn.Before(td) {
						d.SendApproachingWhatsApp(ch)
					} else if tn.Equal(td) {
						d.SendDueTodayWhatsApp(ch)
					}
				}
				if _, err := repo.InsertReminder(ctx, ch.ID, "due_soon", "whatsapp", "sent", body, ptrNow()); err != nil {
					log.Println("[FlowPay Job] insert reminder WA:", err)
				}
			}
		}
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
		subject, body := overdueTemplate(ch, priorOverdue)
		log.Println("[FlowPay Job]", subject)
		if ok, _ := shouldPersist(ctx, repo, ch.ID, "overdue"); ok {
			if shouldSendEmail(ch.ClientFollowupChannel) {
				if d != nil {
					if priorOverdue == 0 {
						d.SendOverdueFirstEmail(ch)
					} else {
						d.SendOverdueFollowUpEmail(ch)
					}
				}
				emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subject, body)
				if _, err := repo.InsertReminder(ctx, ch.ID, "overdue", "email", "sent", emailMessage, ptrNow()); err != nil {
					log.Println("[FlowPay Job] insert reminder:", err)
				}
			}
			if shouldSendWhatsApp(ch.ClientFollowupChannel) {
				if d != nil {
					if priorOverdue == 0 {
						d.SendOverdueFirstWhatsApp(ch)
					} else {
						d.SendOverdueFollowUpWhatsApp(ch)
					}
				}
				if _, err := repo.InsertReminder(ctx, ch.ID, "overdue", "whatsapp", "sent", body, ptrNow()); err != nil {
					log.Println("[FlowPay Job] insert reminder WA:", err)
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

func shouldPersist(ctx context.Context, repo *repository.Repository, chargeID int64, kind string) (bool, error) {
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

