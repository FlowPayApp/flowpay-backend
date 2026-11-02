package controller

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// @Summary		Listar cobros
// @Description	Cobros de la empresa del token. Un member solo ve los de su cartera.
// @Tags			Cobros
// @Produce		json
// @Security		BearerAuth
// @Success		200	{array}		service.ChargeDTO
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/charges [get]
func (d *Deps) ListCharges(c *gin.Context) {
	list, err := d.Svc.ListCharges(c.Request.Context(), d.companyID(c), d.memberUID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// @Summary		Crear cobro
// @Description	due_date en formato YYYY-MM-DD.
// @Tags			Cobros
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			body	body		service.CreateChargeInput	true	"Datos del cobro"
// @Success		201		{object}	idResponse
// @Failure		400		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/charges [post]
func (d *Deps) CreateCharge(c *gin.Context) {
	var in service.CreateChargeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	id, err := d.Svc.CreateCharge(c.Request.Context(), d.companyID(c), d.memberUID(c), in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// @Summary		Actualizar cobro
// @Description	Campos opcionales. set_paid true marca cobrado hoy; false lo reabre y quita los pagos. Devuelve el cobro actualizado.
// @Tags			Cobros
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int							true	"ID del cobro"
// @Param			body	body		service.PatchChargeInput	true	"Campos a cambiar"
// @Success		200		{object}	service.ChargeDTO
// @Failure		400		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/charges/{id} [patch]
func (d *Deps) PatchCharge(c *gin.Context) {
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
	if err := d.Svc.PatchCharge(c.Request.Context(), d.companyID(c), id, d.memberUID(c), d.userID(c), in); err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ch, err := d.Svc.GetCharge(c.Request.Context(), d.companyID(c), id, d.memberUID(c))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusOK, ch)
}

// @Summary	Obtener cobro
// @Tags		Cobros
// @Produce	json
// @Security	BearerAuth
// @Param		id	path		int	true	"ID del cobro"
// @Success	200	{object}	service.ChargeDTO
// @Failure	400	{object}	errorResponse
// @Failure	404	{object}	errorResponse
// @Failure	500	{object}	errorResponse
// @Failure	401	{object}	errorResponse
// @Router		/api/charges/{id} [get]
func (d *Deps) GetCharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	ch, err := d.Svc.GetCharge(c.Request.Context(), d.companyID(c), id, d.memberUID(c))
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

// @Summary		Eliminar cobro
// @Description	Solo el administrador de la empresa.
// @Tags			Cobros
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		int	true	"ID del cobro"
// @Success		200	{object}	okResponse
// @Failure		400	{object}	errorResponse
// @Failure		403	{object}	errorResponse
// @Failure		404	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/charges/{id} [delete]
func (d *Deps) DeleteCharge(c *gin.Context) {
	if !d.requireCompanyAdmin(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := d.Svc.DeleteCharge(c.Request.Context(), d.companyID(c), id); err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// @Summary		Historial de recordatorios
// @Description	Recordatorios enviados para el cobro (línea de tiempo).
// @Tags			Cobros
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		int	true	"ID del cobro"
// @Success		200	{array}		repository.Reminder
// @Failure		400	{object}	errorResponse
// @Failure		404	{object}	errorResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/charges/{id}/reminders [get]
func (d *Deps) ListReminders(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	list, err := d.Svc.ListReminders(c.Request.Context(), d.companyID(c), id, d.memberUID(c))
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

type replyWhatsAppBody struct {
	Text string `json:"text"`
}

// @Summary		Responder por WhatsApp
// @Description	Solo dentro de la ventana de 24 h desde el último mensaje del cliente (409 si no).
// @Tags			WhatsApp
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int					true	"ID del cobro"
// @Param			body	body		replyWhatsAppBody	true	"Texto"
// @Success		200		{object}	model.Message
// @Failure		400		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		409		{object}	errorResponse
// @Failure		502		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/charges/{id}/whatsapp [post]
func (d *Deps) ReplyChargeWhatsApp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var body replyWhatsAppBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "json inválido"})
		return
	}
	m, err := d.Svc.ReplyChargeWhatsApp(c.Request.Context(), d.companyID(c), id, d.memberUID(c), body.Text)
	if err != nil {
		switch {
		case service.ErrNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrWhatsAppReplyEmpty), errors.Is(err, service.ErrWhatsAppReplyLong):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrWhatsAppReplyWindow), errors.Is(err, service.ErrWhatsAppReplyPhone):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, m)
}

// @Summary	Mensajes de WhatsApp del cobro
// @Tags		WhatsApp
// @Produce	json
// @Security	BearerAuth
// @Param		id	path		int	true	"ID del cobro"
// @Success	200	{array}		model.Message
// @Failure	400	{object}	errorResponse
// @Failure	404	{object}	errorResponse
// @Failure	500	{object}	errorResponse
// @Failure	401	{object}	errorResponse
// @Router		/api/charges/{id}/inbound-whatsapp [get]
func (d *Deps) ListChargeInboundWhatsApp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	list, err := d.Svc.ListChargeInboundWhatsApp(c.Request.Context(), d.companyID(c), id, d.memberUID(c))
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

type simulateInboundBody struct {
	Text string `json:"text"`
}

// @Summary		Simular mensaje entrante
// @Description	Solo disponible si FLOWPAY_ALLOW_WHATSAPP_SIMULATE está activo (desarrollo).
// @Tags			WhatsApp
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int					true	"ID del cobro"
// @Param			body	body		simulateInboundBody	false	"Texto"
// @Success		200		{object}	model.Message
// @Failure		400		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		500		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/charges/{id}/inbound-whatsapp/simulate [post]
func (d *Deps) SimulateChargeInboundWhatsApp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var body simulateInboundBody
	_ = c.ShouldBindJSON(&body)
	m, err := d.Svc.SimulateChargeInboundWhatsApp(c.Request.Context(), d.companyID(c), id, d.memberUID(c), body.Text)
	if err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

// @Summary		Medios de aviso asignados
// @Description	Qué canales (correo, WhatsApp) dejó habilitados el superadmin.
// @Tags			Empresa
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	service.CompanyChannels
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/company/channels [get]
func (d *Deps) GetCompanyChannels(c *gin.Context) {
	out, err := d.Svc.CompanyChannels(c.Request.Context(), d.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudieron leer los medios de aviso."})
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary	Configuración de mensajes
// @Tags		Empresa
// @Produce	json
// @Security	BearerAuth
// @Success	200	{object}	service.MessagingSettingsResponse
// @Failure	500	{object}	errorResponse
// @Failure	401	{object}	errorResponse
// @Router		/api/company/messaging [get]
func (d *Deps) GetCompanyMessaging(c *gin.Context) {
	out, err := d.Svc.GetCompanyMessagingSettings(c.Request.Context(), d.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary		Guardar configuración de mensajes
// @Description	Solo el administrador de la empresa. reminder_policy nula deja la frecuencia como está.
// @Tags			Empresa
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			body	body		service.SaveMessagingInput	true	"Configuración"
// @Success		200		{object}	okResponse
// @Failure		400		{object}	errorResponse
// @Failure		403		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/company/messaging [put]
func (d *Deps) PutCompanyMessaging(c *gin.Context) {
	if !d.requireCompanyAdmin(c) {
		return
	}
	var body service.SaveMessagingInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return
	}
	if err := d.Svc.SaveCompanyMessagingSettings(c.Request.Context(), d.companyID(c), body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type sendReminderBody struct {
	Channels []string `json:"channels"`
}

// @Summary		Enviar recordatorio ahora
// @Description	Body opcional. Sin channels usa los medios configurados para el cobro. Responde 429 si el canal sigue en espera.
// @Tags			Cobros
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int					true	"ID del cobro"
// @Param			body	body		sendReminderBody	false	"Canales (email, whatsapp)"
// @Success		200		{object}	okResponse
// @Failure		400		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		429		{object}	reminderCooldownResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/charges/{id}/reminders [post]
func (d *Deps) SendReminder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var body sendReminderBody
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "json inválido"})
			return
		}
	}
	if err := d.Svc.SendReminderNow(c.Request.Context(), d.companyID(c), id, d.memberUID(c), body.Channels); err != nil {
		var wait *service.ReminderCooldownError
		switch {
		case service.ErrNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.As(err, &wait):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error(), "channel": wait.Channel, "available_at": wait.Until})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// @Summary		Dashboard de cobranza
// @Description	Métricas de la empresa del token.
// @Tags			Dashboard
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	service.DashboardResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/dashboard [get]
func (d *Deps) Dashboard(c *gin.Context) {
	out, err := d.Svc.Dashboard(c.Request.Context(), d.companyID(c), d.memberUID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary		Resumen de la plataforma
// @Description	Solo platform_admin.
// @Tags			Plataforma
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	service.PlatformOverviewResponse
// @Failure		403	{object}	errorResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/platform/overview [get]
func (d *Deps) PlatformOverview(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	out, err := d.Svc.PlatformOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary		Subir adjunto del cobro
// @Description	PDF o imagen (máx. 8 MB) que se envía junto a los recordatorios por WhatsApp.
// @Tags			Cobros
// @Accept			mpfd
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int		true	"ID del cobro"
// @Param			file	formData	file	true	"Archivo"
// @Success		200		{object}	okResponse
// @Failure		400		{object}	errorResponse
// @Failure		500		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/charges/{id}/attachment [post]
func (d *Deps) UploadChargeAttachment(c *gin.Context) {
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
	err = d.Svc.SaveChargeAttachment(c.Request.Context(), d.companyID(c), id, d.memberUID(c), src, fh.Filename, fh.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func publicAttachmentName(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	if ext == "" {
		return "factura.pdf"
	}
	return "factura." + ext
}

// @Summary		Descargar adjunto de un cobro (público)
// @Description	Enlace que recibe el cliente. Con download=1 se fuerza la descarga.
// @Tags			Público
// @Produce		octet-stream
// @Param			token		path	string	true	"Token del adjunto"
// @Param			download	query	string	false	"1 para descargar"
// @Success		200			{file}	file
// @Failure		404
// @Router			/api/public/attachments/{token} [get]
func (d *Deps) PublicAttachment(c *gin.Context) {
	token := c.Param("token")
	f, mimeType, ext, err := d.Svc.OpenPublicAttachment(c.Request.Context(), token)
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
	name := publicAttachmentName(ext)
	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}
	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", disposition+`; filename="`+name+`"`)
	c.DataFromReader(http.StatusOK, st.Size(), mimeType, f, nil)
}
