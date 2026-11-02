package controller

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

// @Summary		Enlace de pago de la empresa
// @Description	Solo el administrador de la empresa.
// @Tags			Empresa
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	service.PaymentPortalView
// @Failure		403	{object}	errorResponse
// @Failure		404	{object}	errorResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/company/payment-portal [get]
func (d *Deps) GetPaymentPortal(c *gin.Context) {
	if !d.requireCompanyAdmin(c) {
		return
	}
	out, err := d.Svc.CompanyPaymentPortal(c.Request.Context(), d.companyID(c))
	if err != nil {
		writePaymentPortalError(c, err, "No se pudo leer el enlace de pago.")
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary		Guardar enlace de pago
// @Description	Elige el portal de FlowPay o propone una URL propia, que queda pendiente de aprobación.
// @Tags			Empresa
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			body	body		service.SavePaymentPortalInput	true	"Modo y URL"
// @Success		200		{object}	service.PaymentPortalView
// @Failure		400		{object}	errorResponse
// @Failure		403		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		500		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/company/payment-portal [put]
func (d *Deps) PutPaymentPortal(c *gin.Context) {
	if !d.requireCompanyAdmin(c) {
		return
	}
	var body service.SavePaymentPortalInput
	if !decodePaymentPortalBody(c, &body) {
		return
	}
	out, err := d.Svc.SaveCompanyPaymentPortal(c.Request.Context(), d.companyID(c), d.userID(c), body)
	if err != nil {
		writePaymentPortalError(c, err, "No se pudo guardar el enlace de pago.")
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary		Enlaces de pago pendientes de revisión
// @Description	Solo platform_admin.
// @Tags			Plataforma
// @Produce		json
// @Security		BearerAuth
// @Success		200	{array}		service.PendingPaymentPortalView
// @Failure		403	{object}	errorResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/platform/payment-portals [get]
func (d *Deps) ListPendingPaymentPortals(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	out, err := d.Svc.ListPendingPaymentPortals(c.Request.Context())
	if err != nil {
		writePaymentPortalError(c, err, "No se pudieron leer los enlaces pendientes.")
		return
	}
	c.JSON(http.StatusOK, out)
}

// @Summary		Aprobar enlace de pago
// @Description	Solo platform_admin.
// @Tags			Plataforma
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		int	true	"ID de la empresa"
// @Success		200	{object}	okResponse
// @Failure		400	{object}	errorResponse
// @Failure		403	{object}	errorResponse
// @Failure		404	{object}	errorResponse
// @Failure		500	{object}	errorResponse
// @Failure		401	{object}	errorResponse
// @Router			/api/platform/companies/{id}/payment-portal/approve [post]
func (d *Deps) ApprovePaymentPortal(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	id, ok := paymentPortalCompanyID(c)
	if !ok {
		return
	}
	if err := d.Svc.ApproveCompanyPaymentPortal(c.Request.Context(), id, d.userID(c)); err != nil {
		writePaymentPortalError(c, err, "No se pudo aprobar el enlace.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type rejectPaymentPortalBody struct {
	Reason string `json:"reason"`
}

// @Summary		Rechazar enlace de pago
// @Description	Solo platform_admin.
// @Tags			Plataforma
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		int						true	"ID de la empresa"
// @Param			body	body		rejectPaymentPortalBody	true	"Motivo"
// @Success		200		{object}	okResponse
// @Failure		400		{object}	errorResponse
// @Failure		403		{object}	errorResponse
// @Failure		404		{object}	errorResponse
// @Failure		500		{object}	errorResponse
// @Failure		401		{object}	errorResponse
// @Router			/api/platform/companies/{id}/payment-portal/reject [post]
func (d *Deps) RejectPaymentPortal(c *gin.Context) {
	if !d.isPlatformAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sin permisos de platform_admin"})
		return
	}
	id, ok := paymentPortalCompanyID(c)
	if !ok {
		return
	}
	var body rejectPaymentPortalBody
	if !decodePaymentPortalBody(c, &body) {
		return
	}
	if err := d.Svc.RejectCompanyPaymentPortal(c.Request.Context(), id, d.userID(c), body.Reason); err != nil {
		writePaymentPortalError(c, err, "No se pudo rechazar el enlace.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func paymentPortalCompanyID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Empresa inválida."})
		return 0, false
	}
	return id, true
}

func decodePaymentPortalBody(c *gin.Context, dest any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return false
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return false
	}
	return true
}

func writePaymentPortalError(c *gin.Context, err error, fallback string) {
	var invalid *service.PaymentPortalError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": invalid.Error()})
	case errors.Is(err, sql.ErrNoRows):
		c.JSON(http.StatusNotFound, gin.H{"error": "Empresa no encontrada."})
	default:
		log.Printf("enlace de pago: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}
