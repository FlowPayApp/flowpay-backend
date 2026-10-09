package notify

import (
	"strings"

	"github.com/flowpay/flowpay-backend/internal/repository"
)

// Plantillas Utility creadas en Twilio (Content Template Builder).
// Meta las rechaza hasta que el estado pase a Approved.
const (
	TemplateApproaching     = "HX63abf15d20a4436d74ea1d3a7ccd69bf" // cobro_proximo_vencer
	TemplateDueToday        = "HX36dbcc1339f47d43e150f4fe5932d071" // cobro_vence_hoy
	TemplateOverdueFirst    = "HX374ccdb59d3e82b96c89c6971a8c52db" // cobro_vencido
	TemplateOverdueFollowUp = "HX7b3bbe33dc9c1002dff7624c1c846df4" // cobro_seguimiento

	PhaseApproaching     = "approaching"
	PhaseDueToday        = "due_today"
	PhaseOverdueFirst    = "overdue_first"
	PhaseOverdueFollowUp = "overdue_followup"
)

// TemplateSIDs ContentSid por fase. Vacío usa la constante de arriba.
type TemplateSIDs struct {
	Approaching     string
	DueToday        string
	OverdueFirst    string
	OverdueFollowUp string
}

// WhatsAppContent mensaje de plantilla listo para la API de Twilio.
type WhatsAppContent struct {
	ContentSID string
	Variables  map[string]string
	Preview    string
}

// BuildWhatsAppTemplate arma el ContentSid y las variables del cobro.
// {{1}} monto, y según la fase: fecha, tienda y enlace.
func BuildWhatsAppTemplate(phase string, ch repository.Charge, payURL string, sids TemplateSIDs) (WhatsAppContent, bool) {
	amount := contentVar(FormatMoneyCLP(ch.Amount))
	due := contentVar(FormatDueDateSpanish(ch.DueDate))
	store := contentVar(ch.ClientName)
	link := contentVar(payURL)

	switch strings.ToLower(strings.TrimSpace(phase)) {
	case PhaseApproaching:
		return WhatsAppContent{
			ContentSID: sidOr(sids.Approaching, TemplateApproaching),
			Variables:  map[string]string{"1": amount, "2": due, "3": store, "4": link},
			Preview:    previewApproaching(amount, due, store, link),
		}, true
	case PhaseDueToday:
		return WhatsAppContent{
			ContentSID: sidOr(sids.DueToday, TemplateDueToday),
			Variables:  map[string]string{"1": amount, "2": store, "3": link},
			Preview:    previewDueToday(amount, store, link),
		}, true
	case PhaseOverdueFirst:
		return WhatsAppContent{
			ContentSID: sidOr(sids.OverdueFirst, TemplateOverdueFirst),
			Variables:  map[string]string{"1": amount, "2": due, "3": store, "4": link},
			Preview:    previewOverdue(amount, due, store, link),
		}, true
	case PhaseOverdueFollowUp:
		return WhatsAppContent{
			ContentSID: sidOr(sids.OverdueFollowUp, TemplateOverdueFollowUp),
			Variables:  map[string]string{"1": amount, "2": store, "3": link},
			Preview:    previewFollowUp(amount, store, link),
		}, true
	default:
		return WhatsAppContent{}, false
	}
}

func sidOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func contentVar(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	if s == "" {
		return "-"
	}
	return s
}

func previewApproaching(amount, due, store, link string) string {
	return "Hola 👋\n\nTe recordamos que tienes un cobro próximo a vencer:\n\n💰 Monto: " + amount + "\n📅 Vence el " + due + "\n🏪 Tienda: " + store + "\n\nSi necesitas apoyo con el pago, quedamos atentos.\n\nPaga en este enlace: " + link + ". Gracias."
}

func previewDueToday(amount, store, link string) string {
	return "Hola 👋\n\nHoy vence el siguiente cobro:\n\n💰 Monto: " + amount + "\n🏪 Tienda: " + store + "\n\n¿Nos confirmas si el pago está en proceso?\n\nPaga en este enlace: " + link + ". Gracias."
}

func previewOverdue(amount, due, store, link string) string {
	return "Hola 👋\n\nEl siguiente cobro está vencido:\n\n💰 Monto: " + amount + "\n📅 Vencía el " + due + "\n🏪 Tienda: " + store + "\n\nAgradeceríamos nos confirmes estado de pago o fecha estimada.\n\nPaga en este enlace: " + link + ". Gracias."
}

func previewFollowUp(amount, store, link string) string {
	return "Hola 👋\n\nSeguimos con este cobro pendiente:\n\n💰 Monto: " + amount + "\n🏪 Tienda: " + store + "\n\nNecesitamos confirmar fecha de pago para poder coordinar internamente.\n\nPaga en este enlace: " + link + ". Gracias."
}
