package controller

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/flowpay/flowpay-backend/internal/twiliovalidate"
	"github.com/gin-gonic/gin"
)

// TwilioWebhookDeps credenciales y flags para el webhook público.
type TwilioWebhookDeps struct {
	AuthToken               string
	ValidateTwilioSignature bool
	// Bases públicas (p. ej. https://www.geldflus.com): Twilio firma la URL configurada en la consola,
	// que detrás de proxies no siempre coincide con la que reconstruyen los encabezados.
	PublicBaseURLs []string
}

func (d *Deps) ListPlatformWhatsAppNumbers(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	if d.WhatsApp == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "whatsapp no configurado"})
		return
	}
	list, err := d.WhatsApp.ListActiveNumbers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.WhatsAppNumber{}
	}
	c.JSON(http.StatusOK, list)
}

func (d *Deps) PutCompanyWhatsApp(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	if d.WhatsApp == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "whatsapp no configurado"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empresa inválida"})
		return
	}
	var body struct {
		PhoneNumber string `json:"phone_number"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JSON inválido"})
		return
	}
	w, err := d.WhatsApp.AssignCompanyNumber(c.Request.Context(), id, body.PhoneNumber)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidWhatsAppNumber):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrCompanyNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrWhatsAppNumberInUse):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	if w == nil {
		c.JSON(http.StatusOK, gin.H{"company_id": id, "phone_number": ""})
		return
	}
	c.JSON(http.StatusOK, w)
}

func (d *Deps) TwilioWhatsAppWebhook(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("[FlowPay WhatsApp] webhook parse form: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}
	form := c.Request.PostForm

	if d.TwilioWebhook.ValidateTwilioSignature && strings.TrimSpace(d.TwilioWebhook.AuthToken) != "" {
		signedURL, ok := d.twilioSignedURL(c, form)
		if !ok {
			c.Status(http.StatusForbidden)
			return
		}
		log.Printf("[FlowPay WhatsApp] webhook firma OK url=%s", signedURL)
	} else {
		log.Printf("[FlowPay WhatsApp] webhook sin validación de firma (FLOWPAY_TWILIO_VALIDATE_WEBHOOK desactivado o sin token)")
	}

	from := form.Get("From")
	to := form.Get("To")
	body := form.Get("Body")
	media := inboundMedia(form)
	log.Printf("[FlowPay WhatsApp] webhook recibido From=%s To=%s BodyLen=%d Media=%d", from, to, len(body), len(media))

	if d.WhatsApp == nil {
		log.Printf("[FlowPay WhatsApp] webhook: servicio nil")
		c.Status(http.StatusOK)
		return
	}

	err := d.WhatsApp.HandleInbound(c.Request.Context(), from, to, body, media)
	if err != nil {
		if errors.Is(err, service.ErrUnknownWhatsAppTo) {
			log.Printf("[FlowPay WhatsApp] webhook: To no registrado: %s", to)
			c.Status(http.StatusOK)
			return
		}
		log.Printf("[FlowPay WhatsApp] webhook error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusOK)
}

// twilioSignedURL devuelve la URL con la que Twilio firmó la petición, si la firma es válida.
func (d *Deps) twilioSignedURL(c *gin.Context, form url.Values) (string, bool) {
	sig := c.GetHeader("X-Twilio-Signature")
	validator := twiliovalidate.RequestValidator{AuthToken: d.TwilioWebhook.AuthToken}
	candidates := twilioWebhookURLs(c, d.TwilioWebhook.PublicBaseURLs)
	for _, u := range candidates {
		if validator.Validate(sig, u, form) {
			return u, true
		}
	}
	log.Printf("[FlowPay WhatsApp] webhook firma inválida path=%s (probadas: %s)", c.Request.URL.Path, strings.Join(candidates, ", "))
	return "", false
}

// TwilioStatusWebhook recibe los avisos de entrega de los WhatsApp enviados (StatusCallback).
// Siempre exige la firma de Twilio: sin ella cualquiera podría marcar mensajes como leídos.
func (d *Deps) TwilioStatusWebhook(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	form := c.Request.PostForm
	if strings.TrimSpace(d.TwilioWebhook.AuthToken) == "" {
		c.Status(http.StatusForbidden)
		return
	}
	if _, ok := d.twilioSignedURL(c, form); !ok {
		c.Status(http.StatusForbidden)
		return
	}
	if d.WhatsApp == nil {
		c.Status(http.StatusNoContent)
		return
	}
	sid := form.Get("MessageSid")
	status := form.Get("MessageStatus")
	if err := d.WhatsApp.HandleDeliveryStatus(c.Request.Context(), sid, status, form.Get("ErrorCode")); err != nil {
		log.Printf("[FlowPay WhatsApp] estado sid=%s status=%s: %v", sid, status, err)
		c.Status(http.StatusInternalServerError)
		return
	}
	if code := form.Get("ErrorCode"); code != "" {
		log.Printf("[FlowPay WhatsApp] estado sid=%s status=%s error=%s", sid, status, code)
	}
	c.Status(http.StatusNoContent)
}

// Twilio admite hasta 10 adjuntos por mensaje.
const maxInboundMedia = 10

func inboundMedia(form url.Values) []model.MessageMedia {
	n, _ := strconv.Atoi(form.Get("NumMedia"))
	if n > maxInboundMedia {
		n = maxInboundMedia
	}
	out := make([]model.MessageMedia, 0, n)
	for i := 0; i < n; i++ {
		u := strings.TrimSpace(form.Get(fmt.Sprintf("MediaUrl%d", i)))
		if u == "" {
			continue
		}
		out = append(out, model.MessageMedia{
			URL:         u,
			ContentType: strings.TrimSpace(form.Get(fmt.Sprintf("MediaContentType%d", i))),
		})
	}
	return out
}

// Tipos que el navegador puede mostrar sin ejecutar código; el resto se entrega como descarga.
var inlineMediaTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"image/gif":       true,
	"application/pdf": true,
}

func inlineMedia(contentType string) bool {
	base := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return inlineMediaTypes[base] || strings.HasPrefix(base, "audio/") || strings.HasPrefix(base, "video/")
}

func (d *Deps) ChargeInboundMedia(c *gin.Context) {
	chargeID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	msgID, err := strconv.ParseInt(c.Param("msgId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad message id"})
		return
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad index"})
		return
	}
	if d.WhatsApp == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "whatsapp no configurado"})
		return
	}
	body, contentType, size, err := d.WhatsApp.OpenChargeMedia(c.Request.Context(), d.companyID(c), chargeID, d.memberUID(c), msgID, index)
	if err != nil {
		if service.ErrNotFound(err) || errors.Is(err, service.ErrMediaNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		log.Printf("[FlowPay WhatsApp] adjunto charge=%d msg=%d index=%d: %v", chargeID, msgID, index, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "no se pudo obtener el adjunto"})
		return
	}
	defer body.Close()
	disposition := "inline"
	if !inlineMedia(contentType) {
		contentType = "application/octet-stream"
		disposition = "attachment"
	}
	c.DataFromReader(http.StatusOK, size, contentType, body, map[string]string{
		"Content-Disposition":    disposition,
		"Cache-Control":          "private, max-age=3600",
		"X-Content-Type-Options": "nosniff",
	})
}

func twilioWebhookFullURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := c.Request.Host
	if h := c.GetHeader("X-Forwarded-Host"); h != "" {
		parts := strings.Split(h, ",")
		host = strings.TrimSpace(parts[0])
	}
	return scheme + "://" + host + c.Request.URL.Path
}

// twilioWebhookURLs URLs con las que Twilio pudo haber firmado la petición, sin repetir.
func twilioWebhookURLs(c *gin.Context, publicBases []string) []string {
	suffix := c.Request.URL.Path
	if q := c.Request.URL.RawQuery; q != "" {
		suffix += "?" + q
	}
	out := []string{}
	seen := map[string]bool{}
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	for _, base := range publicBases {
		if base = strings.TrimRight(strings.TrimSpace(base), "/"); base != "" {
			add(base + suffix)
		}
	}
	forwarded := twilioWebhookFullURL(c)
	if q := c.Request.URL.RawQuery; q != "" {
		forwarded += "?" + q
	}
	add(forwarded)
	return out
}
