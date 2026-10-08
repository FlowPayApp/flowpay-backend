package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ReminderPolicy cuándo salen los recordatorios automáticos de un cobro.
type ReminderPolicy struct {
	// DaysBefore días antes del vencimiento en que se avisa; 0 es el mismo día del vencimiento.
	DaysBefore []int `json:"days_before"`
	// OverdueEvery cada cuántos días se insiste con un cobro vencido.
	OverdueEvery int `json:"overdue_every"`
	// OverdueMax tope de avisos con el cobro vencido: 0 ninguno, OverdueUnlimited sin tope.
	OverdueMax int `json:"overdue_max"`
}

// OverdueUnlimited insiste hasta que el cobro se pague.
const OverdueUnlimited = -1

// Cómo decide un cobro su frecuencia de recordatorios automáticos.
const (
	ReminderModeCompany = "company"
	ReminderModeCustom  = "custom"
	ReminderModeOff     = "off"
)

const (
	maxDaysBefore   = 60
	maxOverdueEvery = 60
	maxOverdueSends = 100
)

// DefaultReminderPolicy 3 días antes, 1 día antes y el día del vencimiento; vencido, cada 3 días hasta 5 veces.
func DefaultReminderPolicy() ReminderPolicy {
	return ReminderPolicy{DaysBefore: []int{3, 1, 0}, OverdueEvery: 3, OverdueMax: 5}
}

// Normalized valida la política y deja los días sin repetir, de mayor a menor.
func (p ReminderPolicy) Normalized() (ReminderPolicy, error) {
	seen := map[int]bool{}
	days := make([]int, 0, len(p.DaysBefore))
	for _, d := range p.DaysBefore {
		if d < 0 || d > maxDaysBefore {
			return p, fmt.Errorf("los días antes del vencimiento van de 0 a %d", maxDaysBefore)
		}
		if !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(days)))
	if p.OverdueEvery < 1 || p.OverdueEvery > maxOverdueEvery {
		return p, fmt.Errorf("con el cobro vencido se puede insistir cada 1 a %d días", maxOverdueEvery)
	}
	if p.OverdueMax < OverdueUnlimited || p.OverdueMax > maxOverdueSends {
		return p, fmt.Errorf("el máximo de avisos con el cobro vencido va de 0 a %d", maxOverdueSends)
	}
	return ReminderPolicy{DaysBefore: days, OverdueEvery: p.OverdueEvery, OverdueMax: p.OverdueMax}, nil
}

// SendsBefore indica si toca avisar cuando faltan daysUntil días para el vencimiento.
func (p ReminderPolicy) SendsBefore(daysUntil int) bool {
	for _, d := range p.DaysBefore {
		if d == daysUntil {
			return true
		}
	}
	return false
}

// FormatReminderDays guarda los días como texto ("3,1,0").
func FormatReminderDays(days []int) string {
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = strconv.Itoa(d)
	}
	return strings.Join(parts, ",")
}

// ParseReminderDays lee los días guardados con FormatReminderDays.
func ParseReminderDays(s string) []int {
	days := []int{}
	for _, part := range strings.Split(s, ",") {
		if d, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			days = append(days, d)
		}
	}
	return days
}
