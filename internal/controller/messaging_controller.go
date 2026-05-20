package controller

import (
	"net/http"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type MessagingController struct{ Deps }

func (h *MessagingController) Get(c *gin.Context) {
	out, err := h.Svc.GetCompanyMessagingSettings(c.Request.Context(), h.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *MessagingController) Put(c *gin.Context) {
	var body service.SaveMessagingInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return
	}
	if err := h.Svc.SaveCompanyMessagingSettings(c.Request.Context(), h.companyID(c), body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
