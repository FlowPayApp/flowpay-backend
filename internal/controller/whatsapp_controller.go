package controller

import (
	"errors"
	"log"
	"net/http"
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
		sig := c.GetHeader("X-Twilio-Signature")
		fullURL := twilioWebhookFullURL(c)
		ok := twiliovalidate.RequestValidator{AuthToken: d.TwilioWebhook.AuthToken}.Validate(sig, fullURL, form)
		if !ok {
			log.Printf("[FlowPay WhatsApp] webhook firma inválida url=%s", fullURL)
			c.Status(http.StatusForbidden)
			return
		}
		log.Printf("[FlowPay WhatsApp] webhook firma OK url=%s", fullURL)
	} else {
		log.Printf("[FlowPay WhatsApp] webhook sin validación de firma (FLOWPAY_TWILIO_VALIDATE_WEBHOOK desactivado o sin token)")
	}

	from := form.Get("From")
	to := form.Get("To")
	body := form.Get("Body")
	log.Printf("[FlowPay WhatsApp] webhook recibido From=%s To=%s BodyLen=%d", from, to, len(body))

	if d.WhatsApp == nil {
		log.Printf("[FlowPay WhatsApp] webhook: servicio nil")
		c.Status(http.StatusOK)
		return
	}

	err := d.WhatsApp.HandleInbound(c.Request.Context(), from, to, body)
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
