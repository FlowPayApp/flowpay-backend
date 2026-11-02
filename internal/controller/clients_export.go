package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// @Summary	Exportar clientes (CSV)
// @Tags		Empresa
// @Produce	text/csv
// @Security	BearerAuth
// @Success	200	{file}		file	"clientes.csv"
// @Failure	500	{object}	errorResponse
// @Failure	401	{object}	errorResponse
// @Router		/api/company/clients/export [get]
func (d *Deps) ExportClients(c *gin.Context) {
	body, err := d.Svc.ExportClientsCSV(c.Request.Context(), d.companyID(c), d.memberUID(c), d.userID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="clientes.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", body)
}
