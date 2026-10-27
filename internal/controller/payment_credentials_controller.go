package controller

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (d *Deps) GetPaymentCredentials(c *gin.Context) {
	if !d.requireCompanyAdmin(c) {
		return
	}
	out, err := d.Svc.PaymentCredentialStatus(c.Request.Context(), d.companyID(c))
	if err != nil {
		log.Printf("credenciales de pago: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no se pudieron leer las credenciales"})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (d *Deps) PutPaymentCredentials(c *gin.Context) {
	if !d.requireCompanyAdmin(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var body service.SavePaymentCredentialsInput
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return
	}
	out, err := d.Svc.RequestPaymentCredentialChange(c.Request.Context(), d.companyID(c), d.userID(c), body)
	if err != nil {
		writeCredentialError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (d *Deps) PreviewPaymentCredentialChange(c *gin.Context) {
	out, err := d.Svc.PreviewCredentialConfirmation(c.Request.Context(), c.Param("token"))
	if err != nil {
		writeCredentialError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (d *Deps) ConfirmPaymentCredentialChange(c *gin.Context) {
	out, err := d.Svc.ConfirmCredentialChange(c.Request.Context(), c.Param("token"))
	if err != nil {
		writeCredentialError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func writeCredentialError(c *gin.Context, err error) {
	var invalid *service.PaymentCredentialError
	switch {
	case errors.Is(err, service.ErrPaymentCredentialKey):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrCredentialDelivery):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No se pudo enviar la confirmación. Inténtalo de nuevo."})
	case errors.Is(err, service.ErrCredentialExpired):
		c.JSON(http.StatusGone, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrCredentialUsed):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrCredentialLink):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": invalid.Error()})
	default:
		log.Printf("credenciales de pago: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no se pudieron guardar las credenciales"})
	}
}
