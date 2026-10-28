package service

import (
	"bytes"
	"context"
	"encoding/csv"
)

// ExportClientsCSV arma el archivo de clientes de la empresa y deja el registro de auditoría.
func (s *Service) ExportClientsCSV(ctx context.Context, companyID, memberUID, actor int64) ([]byte, error) {
	rows, err := s.Repo.ListClientsForExport(ctx, companyID, memberUID)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"nombre", "email", "telefono", "codigo", "sucursal", "direccion"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := w.Write([]string{
			csvCell(row.Name),
			csvCell(row.Email),
			csvCell(row.Phone),
			csvCell(row.ClientCode),
			csvCell(row.BranchName),
			csvCell(row.Address),
		}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, 0, companyID, auditClientsExported)
	return buf.Bytes(), nil
}

func csvCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
