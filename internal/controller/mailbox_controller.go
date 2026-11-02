package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/model"
	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type companyMailboxBody struct {
	FromName     string `json:"from_name"`
	FromEmail    string `json:"from_email"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     string `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
}

// @Summary		Casillas de correo de las empresas
// @Description	Solo platform_admin.
// @Tags			Plataforma
// @Produce		json
// @Security		BearerAuth
// @Success		200	{array}		model.CompanyMailbox
// @Failure		403	{object}	errorResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/platform/mailboxes [get]
func (d *Deps) ListPlatformMailboxes(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	list, err := d.Svc.ListCompanyMailboxes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.CompanyMailbox{}
	}
	c.JSON(http.StatusOK, list)
}

// @Summary		Configurar casilla SMTP de una empresa
// @Description	Solo platform_admin.
// @Tags			Plataforma
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int					true	"ID de la empresa"
// @Param			body	body		companyMailboxBody	true	"Casilla SMTP"
// @Success		200		{object}	model.CompanyMailbox
// @Failure		400		{object}	errorResponse
// @Failure		403		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		409		{object}	errorResponse
// @Failure		500		{object}	errorResponse
// @Failure		503		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/platform/companies/{id}/mailbox [put]
func (d *Deps) PutCompanyMailbox(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empresa inválida"})
		return
	}
	var body companyMailboxBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JSON inválido"})
		return
	}
	saved, err := d.Svc.SaveCompanyMailbox(c.Request.Context(), id, service.MailboxInput{
		FromName:     body.FromName,
		FromEmail:    body.FromEmail,
		SMTPHost:     body.SMTPHost,
		SMTPPort:     body.SMTPPort,
		SMTPUsername: body.SMTPUsername,
		SMTPPassword: body.SMTPPassword,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidMailbox), errors.Is(err, service.ErrMailboxPasswordRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrMailboxKey):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrCompanyNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrMailboxInUse):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	if saved == nil {
		c.JSON(http.StatusOK, gin.H{"company_id": id, "from_email": ""})
		return
	}
	c.JSON(http.StatusOK, saved)
}
