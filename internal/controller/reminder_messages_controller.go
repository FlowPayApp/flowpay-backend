package controller

import (
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (d *Deps) CreateReminderMessage(c *gin.Context) {
	var in service.CreateReminderMessageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	companyID := d.resolveReminderMessageCompanyID(c, in.CompanyID)
	row, err := d.Svc.CreateReminderMessage(c.Request.Context(), companyID, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (d *Deps) PatchReminderMessage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var in service.PatchReminderMessageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	row, err := d.Svc.PatchReminderMessage(c.Request.Context(), id, d.companyID(c), d.isPlatformAdmin(c), in)
	if err != nil {
		if service.ErrNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (d *Deps) resolveReminderMessageCompanyID(c *gin.Context, bodyCompanyID int64) int64 {
	if d.isPlatformAdmin(c) && bodyCompanyID > 0 {
		return bodyCompanyID
	}
	id := d.companyID(c)
	if id != 0 {
		return id
	}
	return bodyCompanyID
}
