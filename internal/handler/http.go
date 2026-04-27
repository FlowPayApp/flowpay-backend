package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/flowpay/flowpay-backend/internal/services"
	"github.com/gin-gonic/gin"
)

type HTTP struct {
	Svc            *service.Service
	WhatsApp       *services.WhatsAppService
	TwilioWebhook  TwilioWebhookDeps
	DefaultCompany int64
	JWTSecret      string
}

func (h *HTTP) companyID(c *gin.Context) int64 {
	if h.JWTSecret != "" {
		if v, ok := c.Get("company_id"); ok {
			if id, ok := v.(int64); ok {
				return id
			}
		}
		return h.DefaultCompany
	}
	if v := c.Query("company_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	}
	return h.DefaultCompany
}

func (h *HTTP) Register(r *gin.Engine, jwtMiddleware gin.HandlerFunc) {
	r.GET("/api/public/attachments/:token", h.publicAttachment)
	r.POST("/api/webhooks/twilio/whatsapp", h.twilioWhatsAppWebhook)

	api := r.Group("/api")
	api.Use(jwtMiddleware)
	{
		api.GET("/clients", h.listClients)
		api.POST("/clients", h.createClient)
		api.PATCH("/clients/:id", h.patchClient)
		api.GET("/charges", h.listCharges)
		api.POST("/charges", h.createCharge)
		api.GET("/charges/:id", h.getCharge)
		api.PATCH("/charges/:id", h.patchCharge)
		api.GET("/charges/:id/reminders", h.listReminders)
		api.POST("/charges/:id/reminders", h.sendReminder)
		api.POST("/charges/:id/attachment", h.uploadChargeAttachment)
		api.POST("/payments", h.recordPayment)
		api.GET("/dashboard", h.dashboard)
		api.GET("/platform/overview", h.platformOverview)
		api.GET("/messages", h.listWhatsAppMessages)
		api.POST("/messages/send", h.sendWhatsAppMessage)
	}
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func (h *HTTP) isPlatformAdmin(c *gin.Context) bool {
	v, ok := c.Get("role")
	if !ok {
		return false
	}
	role, ok := v.(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(strings.ToLower(role)) == "platform_admin"
}

func (h *HTTP) listClients(c *gin.Context) {
	list, err := h.Svc.ListClients(c.Request.Context(), h.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

type createClientBody struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type patchClientBody struct {
	IsActive        *bool   `json:"is_active"`
	FollowupChannel *string `json:"followup_channel"`
}

func (h *HTTP) createClient(c *gin.Context) {
	var body createClientBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	id, err := h.Svc.CreateClient(c.Request.Context(), h.companyID(c), body.Name, body.Email, body.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *HTTP) patchClient(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var body patchClientBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if body.IsActive == nil && body.FollowupChannel == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "indica is_active y/o followup_channel"})
		return
	}
	if err := h.Svc.PatchClient(c.Request.Context(), h.companyID(c), id, body.IsActive, body.FollowupChannel); err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTP) listCharges(c *gin.Context) {
	list, err := h.Svc.ListCharges(c.Request.Context(), h.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *HTTP) createCharge(c *gin.Context) {
	var in service.CreateChargeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	id, err := h.Svc.CreateCharge(c.Request.Context(), h.companyID(c), in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *HTTP) patchCharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var in service.PatchChargeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if err := h.Svc.PatchCharge(c.Request.Context(), h.companyID(c), id, in); err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ch, err := h.Svc.GetCharge(c.Request.Context(), h.companyID(c), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (h *HTTP) getCharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	ch, err := h.Svc.GetCharge(c.Request.Context(), h.companyID(c), id)
	if err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (h *HTTP) listReminders(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	list, err := h.Svc.ListReminders(c.Request.Context(), h.companyID(c), id)
	if err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *HTTP) sendReminder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := h.Svc.SendReminderNow(c.Request.Context(), h.companyID(c), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type paymentBody struct {
	ChargeID int64   `json:"charge_id"`
	Amount   float64 `json:"amount"`
}

func (h *HTTP) recordPayment(c *gin.Context) {
	var body paymentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if err := h.Svc.RecordPayment(c.Request.Context(), h.companyID(c), body.ChargeID, body.Amount); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func (h *HTTP) dashboard(c *gin.Context) {
	d, err := h.Svc.Dashboard(c.Request.Context(), h.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *HTTP) platformOverview(c *gin.Context) {
	if !h.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	d, err := h.Svc.PlatformOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *HTTP) uploadChargeAttachment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart"})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "se requiere campo file"})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()
	err = h.Svc.SaveChargeAttachment(c.Request.Context(), h.companyID(c), id, src, fh.Filename, fh.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HTTP) publicAttachment(c *gin.Context) {
	token := c.Param("token")
	f, mimeType, err := h.Svc.OpenPublicAttachment(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", `inline; filename="cobro"`)
	c.DataFromReader(http.StatusOK, st.Size(), mimeType, f, nil)
}
