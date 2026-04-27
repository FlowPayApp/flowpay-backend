package domain

import "time"

// ChargeStatus refleja la regla de negocio del MVP (cobro: pendiente / vencido / cobrado).
func ChargeStatus(paidAt *time.Time, dueDate time.Time, today time.Time) string {
	if paidAt != nil {
		return "paid"
	}
	t0 := truncateDate(today)
	d0 := truncateDate(dueDate)
	if t0.Before(d0) {
		return "pending"
	}
	if t0.After(d0) {
		return "overdue"
	}
	return "pending"
}

func truncateDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// RiskLevel: heurística por cobros vencidos sin pagar del cliente.
func RiskLevel(overdueCount int, overdueAmount float64) string {
	if overdueCount >= 2 || overdueAmount >= 500000 {
		return "high"
	}
	if overdueCount == 1 || overdueAmount >= 150000 {
		return "medium"
	}
	return "low"
}
