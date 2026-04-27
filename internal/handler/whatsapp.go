package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/repository"
	"github.com/flowpay/flowpay-backend/internal/services"
	"github.com/flowpay/flowpay-backend/internal/twiliovalidate"
	"github.com/gin-gonic/gin"
)

// TwilioWebhookDeps credenciales y flags para el webhook público.
type TwilioWebhookDeps struct {
	AuthToken               string
	ValidateTwilioSignature bool
}

func (h *HTTP) listWhatsAppMessages(c *gin.Context) {
	if h.WhatsApp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "whatsapp no configurado"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	phone := c.Query("phone")
	list, err := h.WhatsApp.ListMessages(c.Request.Context(), h.companyID(c), limit, offset, phone)
	if err != nil {
		log.Printf("[FlowPay WhatsApp] list messages: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

type sendWhatsAppBody struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

func (h *HTTP) sendWhatsAppMessage(c *gin.Context) {
	if h.WhatsApp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "whatsapp no configurado"})
		return
	}
	var body sendWhatsAppBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	err := h.WhatsApp.SendMessage(c.Request.Context(), h.companyID(c), body.To, body.Message)
	if err != nil {
		if errors.Is(err, repository.ErrNoActiveWhatsAppNumber) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[FlowPay WhatsApp] send: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTP) twilioWhatsAppWebhook(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("[FlowPay WhatsApp] webhook parse form: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}
	form := c.Request.PostForm

	if h.TwilioWebhook.ValidateTwilioSignature && strings.TrimSpace(h.TwilioWebhook.AuthToken) != "" {
		sig := c.GetHeader("X-Twilio-Signature")
		fullURL := twilioWebhookFullURL(c)
		ok := twiliovalidate.RequestValidator{AuthToken: h.TwilioWebhook.AuthToken}.Validate(sig, fullURL, form)
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

	if h.WhatsApp == nil {
		log.Printf("[FlowPay WhatsApp] webhook: servicio nil")
		c.Status(http.StatusOK)
		return
	}

	err := h.WhatsApp.HandleInbound(c.Request.Context(), from, to, body)
	if err != nil {
		if errors.Is(err, services.ErrUnknownWhatsAppTo) {
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
