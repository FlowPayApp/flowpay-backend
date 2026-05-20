package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type DashboardController struct{ Deps }

func (h *DashboardController) Get(c *gin.Context) {
	d, err := h.Svc.Dashboard(c.Request.Context(), h.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *DashboardController) PlatformOverview(c *gin.Context) {
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
