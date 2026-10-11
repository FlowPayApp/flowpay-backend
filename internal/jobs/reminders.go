package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/domain"
	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/remindercontent"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// Schedule hora del día a la que corre el ciclo de recordatorios automáticos.
type Schedule struct {
	Hour     int
	Minute   int
	Location *time.Location
}

// Clock la hora como HH:MM.
func (s Schedule) Clock() string {
	return fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
}

func (s Schedule) String() string {
	return fmt.Sprintf("todos los días a las %s (%s)", s.Clock(), s.Location)
}

const (
	reminderJobName = "reminders"
	// catchUpWindow si el API estaba caído a la hora programada, el ciclo del día aún corre dentro de este margen.
	catchUpWindow = 2 * time.Hour
)

// StartReminderJob corre una vez al día, a la hora de sched, para **todas** las empresas en `companies`.
// Arrancar o desplegar el API no dispara envíos: el día queda reservado en job_runs y
// solo una ejecución (aunque haya varias instancias) lo procesa.
func StartReminderJob(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, sched Schedule, appPublicURL string) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		var settled string
		check := func() {
			now := time.Now().In(sched.Location)
			day := now.Format("2006-01-02")
			if day == settled {
				return
			}
			slot := time.Date(now.Year(), now.Month(), now.Day(), sched.Hour, sched.Minute, 0, 0, sched.Location)
			if now.Before(slot) {
				return
			}
			if now.Sub(slot) > catchUpWindow {
				settled = day
				return
			}
			claimed, err := repo.ClaimDailyJob(context.Background(), reminderJobName, day)
			if err != nil {
				log.Println("[FlowPay Job] no se pudo reservar el ciclo del día:", err)
				return
			}
			settled = day
			if !claimed {
				log.Printf("[FlowPay Job] los recordatorios de %s ya se procesaron", day)
				return
			}
			runAllCompanies(repo, d, appPublicURL, truncate(now))
		}
		check()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	}()
}

func runAllCompanies(repo *repository.DB, d *notify.Dispatcher, appPublicURL string, today time.Time) {
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
		runOnce(context.Background(), repo, d, cid, appPublicURL, today)
	}
}

// sender lo que comparten los envíos de una empresa en un ciclo.
type sender struct {
	repo         *repository.DB
	d            *notify.Dispatcher
	mailbox      *notify.SMTPConfig
	waFrom       string
	appPublicURL string
}

// runOnce avisa a cada cobro sin pagar según su frecuencia (la propia o la de la empresa).
// today es la fecha del ciclo, a medianoche en la zona del horario.
func runOnce(ctx context.Context, repo *repository.DB, d *notify.Dispatcher, companyID int64, appPublicURL string, today time.Time) {
	log.Printf("[FlowPay Job] Inicio de ciclo de recordatorios (company_id=%d)…", companyID)
	waFrom, waErr := repo.FirstActiveWhatsAppToForCompany(ctx, companyID)
	if waErr != nil && !errors.Is(waErr, sql.ErrNoRows) {
		log.Println("[FlowPay Job] número WhatsApp:", waErr)
	}
	if waErr != nil {
		waFrom = ""
	}
	s := sender{repo: repo, d: d, mailbox: companyMailbox(ctx, repo, companyID), waFrom: waFrom, appPublicURL: appPublicURL}

	companyPolicy, err := repo.GetCompanyReminderPolicy(ctx, companyID)
	if err != nil {
		log.Println("[FlowPay Job] frecuencia de la empresa:", err)
		companyPolicy = domain.DefaultReminderPolicy()
	}
	charges, err := repo.ChargesForAutoReminders(ctx, companyID)
	if err != nil {
		log.Println("[FlowPay Job] error listando cobros:", err)
		return
	}
	for _, item := range charges {
		ch := item.Charge
		channel := item.Reminders.Channel
		if channel == "" {
			channel = ch.ClientFollowupChannel
		}
		if !allowAutoFollowUp(channel) {
			continue
		}
		policy := companyPolicy
		if item.Reminders.Mode == domain.ReminderModeCustom {
			policy = item.Reminders.Policy
		}
		due := time.Date(ch.DueDate.Year(), ch.DueDate.Month(), ch.DueDate.Day(), 0, 0, 0, 0, today.Location())

		if !today.After(due) {
			daysUntil := remindercontent.CalendarDaysUntilDue(today, due)
			if !policy.SendsBefore(daysUntil) {
				continue
			}
			if ok, _ := shouldPersist(ctx, repo, ch.ID, "due_soon"); !ok {
				continue
			}
			phase := remindercontent.PhaseApproaching
			if daysUntil == 0 {
				phase = remindercontent.PhaseDueToday
			}
			subject, body, _, payURL, err := remindercontent.ResolveReminder(ctx, repo, ch.CompanyID, phase, daysUntil, 0, ch, appPublicURL)
			if err != nil {
				log.Println("[FlowPay Job] resolve template due_soon:", err)
				subject, body = dueSoonTemplate(ch, today, due)
			}
			s.send(ctx, ch, channel, "due_soon", phase, subject, body, payURL)
			continue
		}

		if policy.OverdueMax == 0 {
			continue
		}
		sends, last, err := repo.OverdueReminderHistory(ctx, ch.ID)
		if err != nil {
			log.Println("[FlowPay Job] historial de avisos de mora:", err)
			continue
		}
		if policy.OverdueMax != domain.OverdueUnlimited && sends >= policy.OverdueMax {
			continue
		}
		if last != nil && calendarDays(truncate(last.In(today.Location())), today) < policy.OverdueEvery {
			continue
		}
		phase := remindercontent.PhaseOverdueFollowUp
		if sends == 0 {
			phase = remindercontent.PhaseOverdueFirst
		}
		subject, body, _, payURL, err := remindercontent.ResolveReminder(ctx, repo, ch.CompanyID, phase, 0, sends, ch, appPublicURL)
		if err != nil {
			log.Println("[FlowPay Job] resolve template overdue:", err)
			subject, body = overdueTemplate(ch, sends)
		}
		s.send(ctx, ch, channel, "overdue", phase, subject, body, payURL)
	}
	log.Printf("[FlowPay Job] Ciclo completado (company_id=%d).", companyID)
}

// send envía por los canales de channel y registra cada envío logrado.
func (s sender) send(ctx context.Context, ch repository.Charge, channel, kind, phase, subject, body, payURL string) {
	log.Println("[FlowPay Job]", subject)
	if shouldSendEmail(channel) {
		if sendCompanyEmail(s.d, ch, subject, body, s.mailbox) {
			emailMessage := fmt.Sprintf("Asunto: %s\n\n%s", subject, body)
			if _, err := s.repo.InsertReminder(ctx, ch.ID, kind, "email", "sent", emailMessage, ptrNow()); err != nil {
				log.Println("[FlowPay Job] insert reminder:", err)
			}
		}
	}
	if shouldSendWhatsApp(channel) {
		if preview, sent, ok := sendCompanyWhatsAppTemplate(s.d, ch, s.waFrom, phase, payURL); ok {
			saveWhatsAppReminder(ctx, s.repo, ch.ID, kind, preview, sent)
		}
	}
}

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// calendarDays días de calendario entre dos fechas a medianoche en la misma zona.
func calendarDays(from, to time.Time) int {
	return int(to.Sub(from).Round(24*time.Hour) / (24 * time.Hour))
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

func sendCompanyWhatsAppTemplate(d *notify.Dispatcher, ch repository.Charge, from, phase, payURL string) (string, notify.SentWhatsApp, bool) {
	msg, ok := notify.BuildWhatsAppTemplate(phase, ch, payURL, notify.TemplateSIDs{})
	if !ok {
		log.Println("[FlowPay Job] WhatsApp: fase sin plantilla", phase)
		return "", notify.SentWhatsApp{}, false
	}
	if d == nil {
		return msg.Preview, notify.SentWhatsApp{}, true
	}
	preview, sent, err := d.SendCompanyWhatsAppTemplate(ch, from, phase, payURL)
	if err != nil {
		log.Println("[FlowPay Job] WhatsApp:", err)
		return "", notify.SentWhatsApp{}, false
	}
	if preview == "" {
		preview = msg.Preview
	}
	return preview, sent, true
}

func saveWhatsAppReminder(ctx context.Context, repo *repository.DB, chargeID int64, kind, preview string, sent notify.SentWhatsApp) {
	id, err := repo.InsertReminder(ctx, chargeID, kind, "whatsapp", "sent", preview, ptrNow())
	if err != nil {
		log.Println("[FlowPay Job] insert reminder WA:", err)
		return
	}
	if sent.SID == "" {
		return
	}
	if err := repo.SetReminderDelivery(ctx, id, sent.SID, sent.Status); err != nil {
		log.Println("[FlowPay Job] seguimiento de entrega WA:", err)
	}
}
