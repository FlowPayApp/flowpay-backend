package repository

import "context"

// ClientExportRow datos de un cliente para el archivo de exportación.
type ClientExportRow struct {
	Name       string
	Email      string
	Phone      string
	ClientCode string
	BranchName string
	Address    string
}

// ListClientsForExport clientes visibles para quien exporta. memberUID 0 ve la empresa completa.
func (db *DB) ListClientsForExport(ctx context.Context, companyID, memberUID int64) ([]ClientExportRow, error) {
	rows, err := db.db.QueryContext(ctx, `
SELECT name,
       COALESCE(email, ''),
       COALESCE(phone, ''),
       COALESCE(client_code, ''),
       COALESCE(branch_name, ''),
       COALESCE(address, '')
FROM clients
WHERE company_id = $1
  AND ($2::bigint = 0 OR COALESCE(assigned_to, created_by) = $2)
ORDER BY name ASC, id ASC
`, companyID, memberUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientExportRow
	for rows.Next() {
		var row ClientExportRow
		if err := rows.Scan(&row.Name, &row.Email, &row.Phone, &row.ClientCode, &row.BranchName, &row.Address); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if out == nil {
		out = []ClientExportRow{}
	}
	return out, rows.Err()
}
