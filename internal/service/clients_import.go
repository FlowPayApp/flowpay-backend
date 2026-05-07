package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/dberrors"
	"github.com/flowpay/flowpay-backend/internal/repository"
)

// ErrImportHistoryUnavailable la tabla client_import_batches no existe (migración pendiente).
var ErrImportHistoryUnavailable = errors.New("import history table missing")

// ImportDistributorResult resumen de importación planilla FlowPay (formato distribuidor).
type ImportDistributorResult struct {
	Created int                    `json:"created"`
	Updated int                    `json:"updated"`
	Errors  []ImportDistributorErr `json:"errors"`
}

// ImportDistributorErr error por fila (1 = primera fila de datos tras el encabezado).
type ImportDistributorErr struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Cabeceras fijas obligatorias (orden libre; se compara en mayúsculas).
// EMAIL es opcional (planillas antiguas de 6 columnas sin email siguen siendo válidas).
// Método de pago: MPAGO o CPAGO (legado), no ambas.
var fixedDistributorImportHeaders = []string{
	"CODIGO", "SUCURSAL", "NOMBRE", "DIRECCION", "TELEFONO",
}

var distributorHeaderSet = func() map[string]struct{} {
	m := make(map[string]struct{}, 10)
	for _, h := range fixedDistributorImportHeaders {
		m[h] = struct{}{}
	}
	m["EMAIL"] = struct{}{}
	m["MPAGO"] = struct{}{}
	m["CPAGO"] = struct{}{} // legacy: misma semántica que MPAGO
	return m
}()

const maxImportDataRows = 10000

// ImportClientsDistributorRows importa desde matriz de filas (primera fila = cabecera). El front envía filas leídas de Excel.
func (s *Service) ImportClientsDistributorRows(ctx context.Context, companyID int64, rows [][]string) (*ImportDistributorResult, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("sin filas")
	}
	header := rows[0]
	idx, err := validateDistributorHeader(header)
	if err != nil {
		return nil, err
	}
	if len(rows) > maxImportDataRows+1 {
		return nil, fmt.Errorf("demasiadas filas (máximo %d datos; tienes %d)", maxImportDataRows, len(rows)-1)
	}

	out := &ImportDistributorResult{Errors: []ImportDistributorErr{}}
	const maxErrors = 80
	lineNum := 1

	for _, rec := range rows[1:] {
		lineNum++
		if rowEmpty(rec) {
			continue
		}
		codigo := getCol(rec, idx, "CODIGO")
		sucursal := getCol(rec, idx, "SUCURSAL")
		nombre := getCol(rec, idx, "NOMBRE")
		dir := getCol(rec, idx, "DIRECCION")
		tel := getCol(rec, idx, "TELEFONO")
		email := getCol(rec, idx, "EMAIL")
		mpago := getPaymentCol(rec, idx)

		ext := buildExternalCode(sucursal, codigo)
		if ext == "" {
			if len(out.Errors) < maxErrors {
				out.Errors = append(out.Errors, ImportDistributorErr{Line: lineNum, Message: "CODIGO/SUCURSAL: se necesita al menos CODIGO para identificar el cliente"})
			}
			continue
		}
		if strings.TrimSpace(nombre) == "" {
			if len(out.Errors) < maxErrors {
				out.Errors = append(out.Errors, ImportDistributorErr{Line: lineNum, Message: "NOMBRE obligatorio"})
			}
			continue
		}

		inserted, err := s.Repo.UpsertClientImport(ctx, companyID, repository.ClientImportFields{
			Name:         nombre,
			Email:        email,
			Phone:        tel,
			Address:      dir,
			IsActive:     true,
			ExternalCode: ext,
			ClientCode:   codigo,
			BranchName:   sucursal,
			PaymentTerms: mpago,
		})
		if err != nil {
			if len(out.Errors) < maxErrors {
				out.Errors = append(out.Errors, ImportDistributorErr{Line: lineNum, Message: err.Error()})
			}
			continue
		}
		if inserted {
			out.Created++
		} else {
			out.Updated++
		}
	}
	return out, nil
}

func normHeader(s string) string {
	return strings.TrimSpace(strings.ToUpper(strings.Trim(s, "\t\r ")))
}

func validateDistributorHeader(header []string) (map[string]int, error) {
	idx := make(map[string]int, 10)
	nonEmpty := 0
	for i, h := range header {
		key := normHeader(h)
		if key == "" {
			continue
		}
		nonEmpty++
		if _, ok := distributorHeaderSet[key]; !ok {
			return nil, fmt.Errorf("columna no permitida %q; solo: CODIGO, SUCURSAL, NOMBRE, DIRECCION, TELEFONO, EMAIL (opcional), MPAGO o CPAGO (antiguo)", strings.TrimSpace(h))
		}
		if _, dup := idx[key]; dup {
			return nil, fmt.Errorf("columna repetida: %q", key)
		}
		idx[key] = i
	}
	const maxCols = 7
	if nonEmpty > maxCols {
		return nil, fmt.Errorf("demasiadas columnas en la cabecera (máximo %d)", maxCols)
	}
	for _, want := range fixedDistributorImportHeaders {
		if _, ok := idx[want]; !ok {
			return nil, fmt.Errorf("falta la columna obligatoria %q en la primera fila", want)
		}
	}
	_, hasMP := idx["MPAGO"]
	_, hasCP := idx["CPAGO"]
	if !hasMP && !hasCP {
		return nil, fmt.Errorf("falta la columna MPAGO (método de pago); en archivos antiguos puede llamarse CPAGO")
	}
	if hasMP && hasCP {
		return nil, fmt.Errorf("no puede haber columnas MPAGO y CPAGO a la vez; dejá solo MPAGO")
	}
	return idx, nil
}

func getPaymentCol(rec []string, idx map[string]int) string {
	if i, ok := idx["MPAGO"]; ok && i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	if i, ok := idx["CPAGO"]; ok && i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	return ""
}

func getCol(rec []string, idx map[string]int, col string) string {
	i, ok := idx[col]
	if !ok || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

func rowEmpty(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func buildExternalCode(sucursal, codigo string) string {
	su := strings.TrimSpace(sucursal)
	co := strings.TrimSpace(codigo)
	if co == "" {
		return ""
	}
	if su == "" {
		return co
	}
	return su + "|" + co
}

// ClientImportBatchListItem resumen para listado de cargas.
type ClientImportBatchListItem struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Source       string    `json:"source"`
	Filename     *string   `json:"filename,omitempty"`
	CreatedCount int       `json:"created_count"`
	UpdatedCount int       `json:"updated_count"`
	ErrorCount   int       `json:"error_count"`
}

// ClientImportBatchDetail incluye errores por fila almacenados.
type ClientImportBatchDetail struct {
	ClientImportBatchListItem
	Errors []ImportDistributorErr `json:"errors"`
}

// RecordClientImportBatch persiste el resultado de una importación para el historial.
func (s *Service) RecordClientImportBatch(ctx context.Context, companyID int64, userID *int64, source, filename string, res *ImportDistributorResult) (int64, error) {
	var errJSON []byte
	if len(res.Errors) > 0 {
		var err error
		errJSON, err = json.Marshal(res.Errors)
		if err != nil {
			return 0, err
		}
	}
	return s.Repo.InsertClientImportBatch(ctx, companyID, userID, source, filename, res.Created, res.Updated, len(res.Errors), errJSON)
}

// ListClientImportBatches devuelve cargas recientes de la empresa.
func (s *Service) ListClientImportBatches(ctx context.Context, companyID int64) ([]ClientImportBatchListItem, error) {
	rows, err := s.Repo.ListClientImportBatches(ctx, companyID)
	if err != nil {
		if dberrors.IsUndefinedTable(err) {
			return []ClientImportBatchListItem{}, nil
		}
		return nil, err
	}
	out := make([]ClientImportBatchListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, ClientImportBatchListItem{
			ID:           r.ID,
			CreatedAt:    r.CreatedAt,
			Source:       r.Source,
			Filename:     r.Filename,
			CreatedCount: r.CreatedCount,
			UpdatedCount: r.UpdatedCount,
			ErrorCount:   r.ErrorCount,
		})
	}
	return out, nil
}

// GetClientImportBatch devuelve una carga con el detalle de errores.
func (s *Service) GetClientImportBatch(ctx context.Context, companyID, batchID int64) (*ClientImportBatchDetail, error) {
	r, err := s.Repo.GetClientImportBatch(ctx, companyID, batchID)
	if err != nil {
		if dberrors.IsUndefinedTable(err) {
			return nil, ErrImportHistoryUnavailable
		}
		return nil, err
	}
	d := &ClientImportBatchDetail{
		ClientImportBatchListItem: ClientImportBatchListItem{
			ID:           r.ID,
			CreatedAt:    r.CreatedAt,
			Source:       r.Source,
			Filename:     r.Filename,
			CreatedCount: r.CreatedCount,
			UpdatedCount: r.UpdatedCount,
			ErrorCount:   r.ErrorCount,
		},
		Errors: []ImportDistributorErr{},
	}
	if r.ErrorsJSON != nil && *r.ErrorsJSON != "" {
		if err := json.Unmarshal([]byte(*r.ErrorsJSON), &d.Errors); err != nil {
			return nil, err
		}
	}
	return d, nil
}
