package notify

import (
	"log"
	"time"

	"github.com/flowpay/flowpay-backend/internal/repository"
)

// Dispatcher envía correo y WhatsApp reales si hay credenciales; si no, solo registra en log.
type Dispatcher struct {
	smtp             *SMTPConfig
	emailOverride    string
	twilio           *TwilioConfig
	whatsAppOverride string
	// PublicBaseURL sin barra final; Twilio descarga adjuntos desde URLs públicas (HTTPS).
	publicBaseURL string
}

type Options struct {
	SMTP             *SMTPConfig
	EmailOverride    string
	Twilio           *TwilioConfig
	WhatsAppOverride string
	PublicBaseURL    string
}

func NewDispatcher(opt Options) *Dispatcher {
	d := &Dispatcher{
		smtp:             opt.SMTP,
		emailOverride:    opt.EmailOverride,
		twilio:           opt.Twilio,
		whatsAppOverride: opt.WhatsAppOverride,
		publicBaseURL:    opt.PublicBaseURL,
	}
	if d.smtp != nil && d.smtp.Port == "" {
		d.smtp.Port = "587"
	}
	return d
}

func (d *Dispatcher) whatsAppMediaURLs(ch repository.Charge) []string {
	if d.publicBaseURL == "" {
		return nil
	}
	if ch.AttachmentToken == nil || ch.AttachmentExt == nil || *ch.AttachmentToken == "" || *ch.AttachmentExt == "" {
		return nil
	}
	u := d.publicBaseURL + "/api/public/attachments/" + *ch.AttachmentToken
	return []string{u}
}

func (d *Dispatcher) resolveEmail(client *string) string {
	if d.emailOverride != "" {
		return d.emailOverride
	}
	if client != nil && *client != "" {
		return *client
	}
	return ""
}

func (d *Dispatcher) resolveWhatsApp(clientPhone *string) string {
	if d.whatsAppOverride != "" {
		return NormalizeWhatsAppForTwilio(d.whatsAppOverride)
	}
	if clientPhone != nil && *clientPhone != "" {
		return NormalizeWhatsAppForTwilio(*clientPhone)
	}
	return ""
}

func (d *Dispatcher) resolveSMS(clientPhone *string) string {
	if clientPhone != nil && *clientPhone != "" {
		return normalizeSMSPhone(*clientPhone)
	}
	return ""
}

func dateOnly(t time.Time) time.Time {
	y, m, day := t.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, t.Location())
}

// SendApproaching antes del día de vencimiento (mensaje leve).
func (d *Dispatcher) SendApproaching(ch repository.Charge) {
	body := BodyApproaching(ch)
	subj := "Recordatorio: cobro próximo a vencer"
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendApproachingEmail(ch repository.Charge) {
	body := BodyApproaching(ch)
	subj := "Recordatorio: cobro próximo a vencer"
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
}

func (d *Dispatcher) SendApproachingWhatsApp(ch repository.Charge) {
	body := BodyApproaching(ch)
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendApproachingSMS(ch repository.Charge) {
	body := BodyApproaching(ch)
	d.sendSMS(d.resolveSMS(ch.ClientPhone), body)
}

// SendDueToday el día del vencimiento.
func (d *Dispatcher) SendDueToday(ch repository.Charge) {
	body := BodyDueToday(ch)
	subj := "Hoy vence un cobro pendiente"
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendDueTodayEmail(ch repository.Charge) {
	body := BodyDueToday(ch)
	subj := "Hoy vence un cobro pendiente"
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
}

func (d *Dispatcher) SendDueTodayWhatsApp(ch repository.Charge) {
	body := BodyDueToday(ch)
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendDueTodaySMS(ch repository.Charge) {
	body := BodyDueToday(ch)
	d.sendSMS(d.resolveSMS(ch.ClientPhone), body)
}

// SendOverdueFirst primera notificación de cobro vencido.
func (d *Dispatcher) SendOverdueFirst(ch repository.Charge) {
	body := BodyOverdueFirst(ch)
	subj := "Cobro vencido — acción requerida"
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendOverdueFirstEmail(ch repository.Charge) {
	body := BodyOverdueFirst(ch)
	subj := "Cobro vencido — acción requerida"
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
}

func (d *Dispatcher) SendOverdueFirstWhatsApp(ch repository.Charge) {
	body := BodyOverdueFirst(ch)
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendOverdueFirstSMS(ch repository.Charge) {
	body := BodyOverdueFirst(ch)
	d.sendSMS(d.resolveSMS(ch.ClientPhone), body)
}

// SendOverdueFollowUp seguimiento sobre cobro vencido.
func (d *Dispatcher) SendOverdueFollowUp(ch repository.Charge) {
	body := BodyOverdueFollowUp(ch)
	subj := "Seguimiento de cobro pendiente"
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendOverdueFollowUpEmail(ch repository.Charge) {
	body := BodyOverdueFollowUp(ch)
	subj := "Seguimiento de cobro pendiente"
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
}

func (d *Dispatcher) SendOverdueFollowUpWhatsApp(ch repository.Charge) {
	body := BodyOverdueFollowUp(ch)
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}

func (d *Dispatcher) SendOverdueFollowUpSMS(ch repository.Charge) {
	body := BodyOverdueFollowUp(ch)
	d.sendSMS(d.resolveSMS(ch.ClientPhone), body)
}

// SendManual elige plantilla según fecha y recordatorios de mora previos.
// priorOverdueReminders es COUNT(*) de reminders con kind 'overdue' antes de este envío.
func (d *Dispatcher) SendManual(ch repository.Charge, priorOverdueReminders int, now time.Time) {
	t0 := dateOnly(now)
	t1 := dateOnly(ch.DueDate)
	var body, subj string
	switch {
	case t0.Before(t1):
		body = BodyApproaching(ch)
		subj = "Recordatorio: cobro próximo a vencer"
	case t0.Equal(t1):
		body = BodyDueToday(ch)
		subj = "Hoy vence un cobro pendiente"
	default:
		if priorOverdueReminders == 0 {
			body = BodyOverdueFirst(ch)
			subj = "Cobro vencido — acción requerida"
		} else {
			body = BodyOverdueFollowUp(ch)
			subj = "Seguimiento de cobro pendiente"
		}
	}
	media := d.whatsAppMediaURLs(ch)
	if len(media) == 0 && ch.AttachmentToken != nil && *ch.AttachmentToken != "" {
		log.Printf("[FlowPay] Hay adjunto en cobro #%d pero FLOWPAY_PUBLIC_BASE_URL no está definido: WhatsApp irá sin archivo", ch.ID)
	}
	d.sendEmail(d.resolveEmail(ch.ClientEmail), subj, body)
	d.sendWhatsApp(d.resolveWhatsApp(ch.ClientPhone), body, media)
}
