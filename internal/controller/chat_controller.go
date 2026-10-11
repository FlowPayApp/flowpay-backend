package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (d *Deps) Inbox(c *gin.Context) {
	out, err := d.Svc.Inbox(c.Request.Context(), d.companyID(c), d.memberUID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, out)
}

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
