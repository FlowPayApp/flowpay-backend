package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// @Summary		Bandeja de entrada
// @Description	Conversaciones de WhatsApp de los cobros con mensajes recientes.
// @Tags			Bandeja
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	model.Inbox
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/inbox [get]
func (d *Deps) Inbox(c *gin.Context) {
	out, err := d.Svc.Inbox(c.Request.Context(), d.companyID(c), d.memberUID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, out)
}

// @Summary	Marcar conversación como leída
// @Tags		Bandeja
// @Produce	json
// @Security	BearerAuth
// @Param		id	path		int	true	"ID del cobro"
// @Success	200	{object}	okResponse
// @Failure	400	{object}	errorResponse
// @Failure	404	{object}	errorResponse
// @Failure	500	{object}	errorResponse
// @Failure	401	{object}	errorResponse
// @Router		/api/charges/{id}/read [post]
func (d *Deps) MarkChargeRead(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := d.Svc.MarkChargeRead(c.Request.Context(), d.companyID(c), id, d.memberUID(c)); err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// @Summary	Enviar archivo por WhatsApp
// @Tags		WhatsApp
// @Accept		mpfd
// @Produce	json
// @Security	BearerAuth
// @Param		id		path		int		true	"ID del cobro"
// @Param		file	formData	file	true	"Archivo"
// @Param		text	formData	string	false	"Texto que acompaña al archivo"
// @Success	200		{object}	model.Message
// @Failure	400		{object}	errorResponse
// @Failure	404		{object}	errorResponse
// @Failure	409		{object}	errorResponse
// @Failure	502		{object}	errorResponse
// @Failure	401		{object}	errorResponse
// @Router		/api/charges/{id}/whatsapp/file [post]
func (d *Deps) SendChargeWhatsAppFile(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxChatFileBytes+(1<<20))
	fh, err := c.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrChatFileSize.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "elige un archivo"})
		return
	}
	if fh.Size > service.MaxChatFileBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrChatFileSize.Error()})
		return
	}
	src, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no se pudo leer el archivo"})
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, service.MaxChatFileBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no se pudo leer el archivo"})
		return
	}
	m, err := d.Svc.SendChargeWhatsAppFile(c.Request.Context(), d.companyID(c), id, d.memberUID(c), c.PostForm("text"), fh.Filename, data)
	if err != nil {
		switch {
		case service.ErrNotFound(err):
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		case errors.Is(err, service.ErrChatFileType), errors.Is(err, service.ErrChatFileSize), errors.Is(err, service.ErrWhatsAppReplyLong):
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

// PublicChatFile lo descarga Twilio para entregar el archivo; el token es aleatorio y vence en 24 horas.
//
//	@Summary		Archivo de chat (público)
//	@Description	Lo descarga Twilio para entregar el archivo. El token vence en 24 horas.
//	@Tags			Público
//	@Produce		octet-stream
//	@Param			token	path	string	true	"Token del archivo"
//	@Param			name	path	string	true	"Nombre del archivo"
//	@Success		200		{file}	file
//	@Failure		404
//	@Router			/api/public/chat-files/{token}/{name} [get]
func (d *Deps) PublicChatFile(c *gin.Context) {
	f, err := d.Svc.OpenPublicChatFile(c.Request.Context(), c.Param("token"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Disposition", `inline; filename="`+f.FileName+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, f.ContentType, f.Data)
}
