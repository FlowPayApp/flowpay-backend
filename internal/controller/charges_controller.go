package controller

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (d *Deps) ListCharges(c *gin.Context) {
	list, err := d.Svc.ListCharges(c.Request.Context(), d.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (d *Deps) CreateCharge(c *gin.Context) {
	var in service.CreateChargeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	id, err := d.Svc.CreateCharge(c.Request.Context(), d.companyID(c), in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

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
	if err := d.Svc.PatchCharge(c.Request.Context(), d.companyID(c), id, in); err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ch, err := d.Svc.GetCharge(c.Request.Context(), d.companyID(c), id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (d *Deps) GetCharge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	ch, err := d.Svc.GetCharge(c.Request.Context(), d.companyID(c), id)
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

func (d *Deps) DeleteCharge(c *gin.Context) {
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

func (d *Deps) ListReminders(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	list, err := d.Svc.ListReminders(c.Request.Context(), d.companyID(c), id)
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

func (d *Deps) ListChargeInboundWhatsApp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	list, err := d.Svc.ListChargeInboundWhatsApp(c.Request.Context(), d.companyID(c), id)
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

func (d *Deps) SimulateChargeInboundWhatsApp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var body simulateInboundBody
	_ = c.ShouldBindJSON(&body)
	m, err := d.Svc.SimulateChargeInboundWhatsApp(c.Request.Context(), d.companyID(c), id, body.Text)
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

func (d *Deps) GetCompanyMessaging(c *gin.Context) {
	out, err := d.Svc.GetCompanyMessagingSettings(c.Request.Context(), d.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (d *Deps) PutCompanyMessaging(c *gin.Context) {
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

func (d *Deps) SendReminder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := d.Svc.SendReminderNow(c.Request.Context(), d.companyID(c), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (d *Deps) Dashboard(c *gin.Context) {
	out, err := d.Svc.Dashboard(c.Request.Context(), d.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

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
	err = d.Svc.SaveChargeAttachment(c.Request.Context(), d.companyID(c), id, src, fh.Filename, fh.Size)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (d *Deps) PublicAttachment(c *gin.Context) {
	token := c.Param("token")
	f, mimeType, err := d.Svc.OpenPublicAttachment(c.Request.Context(), token)
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
