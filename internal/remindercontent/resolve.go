package remindercontent

import (
	"context"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/notify"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

const (
	PhaseApproaching     = "approaching"
	PhaseDueToday        = "due_today"
	PhaseOverdueFirst    = "overdue_first"
	PhaseOverdueFollowUp = "overdue_followup"
)

// CalendarDaysUntilDue días calendario desde today hasta due (exclusivo del día de hoy si hoy < due).
func CalendarDaysUntilDue(today, due time.Time) int {
	t0 := dateOnly(today)
	t1 := dateOnly(due)
	if !t0.Before(t1) {
		return 0
	}
	return int(t1.Sub(t0) / (24 * time.Hour))
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func buildPaymentURL(tpl string, ch repository.Charge) string {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		return ""
	}
	s := tpl
	s = strings.ReplaceAll(s, "{{charge_id}}", strconv.FormatInt(ch.ID, 10))
	s = strings.ReplaceAll(s, "{{monto_entero}}", strconv.FormatInt(int64(ch.Amount+0.5), 10))
	s = strings.ReplaceAll(s, "{{client_id}}", strconv.FormatInt(ch.ClientID, 10))
	return s
}

// ApplyPlaceholders sustituye tokens en el cuerpo o asunto.
func ApplyPlaceholders(tpl string, ch repository.Charge, companyName, transferInstructions, paymentURL string) string {
	if tpl == "" {
		return ""
	}
	monto := notify.FormatMoneyCLP(ch.Amount)
	fecha := notify.FormatDueDateSpanish(ch.DueDate)
	s := tpl
	repl := map[string]string{
		"{{monto}}":               "$" + monto,
		"{{monto_sin_signo}}":     monto,
		"{{monto_entero}}":        strconv.FormatInt(int64(ch.Amount+0.5), 10),
		"{{fecha_vencimiento}}":   fecha,
		"{{nombre_sucursal}}":     ch.ClientName,
		"{{datos_transferencia}}": transferInstructions,
		"{{url_pago}}":            paymentURL,
		"{{empresa}}":             companyName,
		"{{charge_id}}":           strconv.FormatInt(ch.ID, 10),
		"{{client_id}}":           strconv.FormatInt(ch.ClientID, 10),
	}
	for k, v := range repl {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}

func pickTemplate(rows []repository.ReminderTemplateRow, phase string, daysUntil int) *repository.ReminderTemplateRow {
	phase = strings.ToLower(strings.TrimSpace(phase))
	var candidates []repository.ReminderTemplateRow
	for _, r := range rows {
		if strings.ToLower(strings.TrimSpace(r.Phase)) != phase {
			continue
		}
		if phase == PhaseApproaching {
			if daysUntil >= r.DayMin && daysUntil <= r.DayMax {
				candidates = append(candidates, r)
			}
		} else {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].SortOrder != candidates[j].SortOrder {
			return candidates[i].SortOrder < candidates[j].SortOrder
		}
		wi := candidates[i].DayMax - candidates[i].DayMin
		wj := candidates[j].DayMax - candidates[j].DayMin
		if wi != wj {
			return wi < wj
		}
		return candidates[i].ID < candidates[j].ID
	})
	return &candidates[0]
}

func defaultSubjectBody(phase string, priorOverdue int, ch repository.Charge) (string, string) {
	switch phase {
	case PhaseApproaching:
		return "Recordatorio: cobro próximo a vencer", notify.BodyApproaching(ch)
	case PhaseDueToday:
		return "Hoy vence un cobro pendiente", notify.BodyDueToday(ch)
	case PhaseOverdueFirst:
		return "Cobro vencido — acción requerida", notify.BodyOverdueFirst(ch)
	case PhaseOverdueFollowUp:
		return "Seguimiento de cobro pendiente", notify.BodyOverdueFollowUp(ch)
	default:
		if priorOverdue == 0 {
			return "Cobro vencido — acción requerida", notify.BodyOverdueFirst(ch)
		}
		return "Seguimiento de cobro pendiente", notify.BodyOverdueFollowUp(ch)
	}
}

// DefaultEmail asunto y texto del sistema para la fase, tal como sale si la empresa no escribe el suyo.
func DefaultEmail(phase string, ch repository.Charge, payURL string) (subject, body string) {
	subject, body = defaultSubjectBody(phase, 0, ch)
	return subject, withPayLink(body, payURL)
}

// PhaseFromCharge clasifica el cobro según la fecha de hoy y recordatorios de mora previos.
func PhaseFromCharge(ch repository.Charge, now time.Time, priorOverdue int) (phase string, daysUntil int) {
	t0 := dateOnly(now)
	t1 := dateOnly(ch.DueDate)
	if t0.Before(t1) {
		return PhaseApproaching, CalendarDaysUntilDue(now, ch.DueDate)
	}
	if t0.Equal(t1) {
		return PhaseDueToday, 0
	}
	if priorOverdue == 0 {
		return PhaseOverdueFirst, 0
	}
	return PhaseOverdueFollowUp, 0
}

// ResolveReminder resuelve el correo y el WhatsApp (plantilla de la empresa o texto del sistema).
// appPublicURL es el origen del frontend; con eso {{url_pago}} es el portal de la sucursal.
func ResolveReminder(ctx context.Context, repo *repository.DB, companyID int64, phase string, daysUntil int, priorOverdue int, ch repository.Charge, appPublicURL string) (emailSubject, emailBody, whatsappBody, payURL string, err error) {
	rows, err := repo.ListReminderTemplates(ctx, companyID)
	if err != nil {
		rows = nil
	}
	cm, err := repo.GetCompanyMessaging(ctx, companyID)
	if err != nil {
		return "", "", "", "", err
	}
	payURL = buildPaymentURL(cm.PaymentURLTemplate, ch)
	if link := clientPortalURL(ctx, repo, appPublicURL, companyID, ch.ClientID); link != "" {
		payURL = link
	}
	defSubject, defBody := defaultSubjectBody(phase, priorOverdue, ch)
	t := pickTemplate(rows, phase, daysUntil)
	if t == nil {
		return defSubject, withPayLink(defBody, payURL), withPayLink(defBody, payURL), payURL, nil
	}
	emailBody = strings.TrimSpace(t.Body)
	if emailBody == "" {
		emailSubject, emailBody = defSubject, defBody
	} else {
		emailBody = ApplyPlaceholders(t.Body, ch, cm.Name, cm.TransferInstructions, payURL)
		emailSubject = strings.TrimSpace(t.EmailSubject)
		if emailSubject == "" {
			emailSubject = defSubject
		} else {
			emailSubject = ApplyPlaceholders(emailSubject, ch, cm.Name, cm.TransferInstructions, payURL)
		}
	}
	whatsappBody = strings.TrimSpace(t.WhatsAppBody)
	if whatsappBody == "" {
		whatsappBody = defBody
	} else {
		whatsappBody = ApplyPlaceholders(t.WhatsAppBody, ch, cm.Name, cm.TransferInstructions, payURL)
	}
	return emailSubject, withPayLink(emailBody, payURL), withPayLink(whatsappBody, payURL), payURL, nil
}

func clientPortalURL(ctx context.Context, repo *repository.DB, appPublicURL string, companyID, clientID int64) string {
	base := strings.TrimRight(strings.TrimSpace(appPublicURL), "/")
	if base == "" || clientID <= 0 {
		return ""
	}
	token, err := repo.EnsureClientPaymentToken(ctx, companyID, clientID)
	if err != nil || strings.TrimSpace(token) == "" {
		log.Printf("[FlowPay] enlace de pago de la sucursal: %v", err)
		return ""
	}
	return base + "/pay/" + url.PathEscape(token)
}

func withPayLink(body, payURL string) string {
	body = strings.TrimSpace(body)
	payURL = strings.TrimSpace(payURL)
	if payURL == "" || strings.Contains(body, payURL) {
		return body
	}
	if body == "" {
		return "Paga aquí: " + payURL
	}
	return body + "\n\nPaga aquí: " + payURL
}
