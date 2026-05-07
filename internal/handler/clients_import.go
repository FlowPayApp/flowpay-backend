package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/flowpay/flowpay-backend/internal/service"
	"github.com/gin-gonic/gin"
)

type importDistributorRowsBody struct {
	Rows     [][]string `json:"rows"`
	Filename *string    `json:"filename"`
}

// POST /api/clients/import-distributor-rows — JSON { "rows": [["CODIGO",...], [...]] } primera fila = cabecera.
func (h *HTTP) clientsImportDistributorRows(c *gin.Context) {
	var body importDistributorRowsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JSON inválido: se espera { \"rows\": [[...]] }"})
		return
	}
	if len(body.Rows) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rows vacío"})
		return
	}
	res, err := h.Svc.ImportClientsDistributorRows(c.Request.Context(), h.companyID(c), body.Rows)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fn := ""
	if body.Filename != nil {
		fn = *body.Filename
	}
	if _, err := h.Svc.RecordClientImportBatch(c.Request.Context(), h.companyID(c), h.jwtUserID(c), "excel", fn, res); err != nil {
		log.Printf("client import batch: %v", err)
	}
	c.JSON(http.StatusOK, res)
}

// GET /api/clients/import-batches
func (h *HTTP) listClientImportBatches(c *gin.Context) {
	list, err := h.Svc.ListClientImportBatches(c.Request.Context(), h.companyID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/clients/import-batches/:id
func (h *HTTP) getClientImportBatch(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id inválido"})
		return
	}
	detail, err := h.Svc.GetClientImportBatch(c.Request.Context(), h.companyID(c), id)
	if err != nil {
		if errors.Is(err, service.ErrImportHistoryUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Falta la tabla de historial (client_import_batches). Ejecutá el DDL en PostgreSQL (postgresql_migration/02_schema.sql) y reiniciá el API.",
			})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no encontrado"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}
