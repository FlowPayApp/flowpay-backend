package dberrors

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PgCode devuelve el código SQLSTATE de PostgreSQL (ej. "23505") o cadena vacía.
func PgCode(err error) string {
	var e *pgconn.PgError
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// IsUniqueViolation detecta violación de unicidad (equivalente a MySQL 1062).
func IsUniqueViolation(err error) bool {
	return PgCode(err) == "23505"
}

// IsUndefinedTable detecta relación inexistente (equivalente a MySQL 1146).
func IsUndefinedTable(err error) bool {
	return PgCode(err) == "42P01"
}
