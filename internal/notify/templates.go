package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flowpay/flowpay-backend/internal/repository"
)

var mesesES = []string{
	"", "enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

func formatDateES(t time.Time) string {
	return fmt.Sprintf("%d de %s", t.Day(), mesesES[int(t.Month())])
}

// FormatDueDateSpanish expone fecha de vencimiento legible (p. ej. plantillas externas).
func FormatDueDateSpanish(t time.Time) string {
	return formatDateES(t)
}

// FormatMoneyCLP expone monto con separador de miles (plantillas / API).
func FormatMoneyCLP(amount float64) string {
	return formatMoneyCL(amount)
}

// formatMoneyCL formatea como 1.200.000 (enteros, separador miles).
func formatMoneyCL(amount float64) string {
	n := int64(amount + 0.5)
	if n < 0 {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteRune('.')
		}
	}
	for i := pre; i < len(s); i += 3 {
		if i > pre {
			b.WriteRune('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func signature(clientName string) string {
	return "— " + clientName
}

// BodyApproaching: antes del día de vencimiento (mensaje leve).
func BodyApproaching(ch repository.Charge) string {
	m := formatMoneyCL(ch.Amount)
	f := formatDateES(ch.DueDate)
	return fmt.Sprintf(`Hola 👋

Te recordamos que tienes un cobro próximo a vencer:

💰 Monto: $%s  
📅 Vence el %s  

Si necesitas apoyo con el pago, quedamos atentos.

%s`, m, f, signature(ch.ClientName))
}

// BodyDueToday: el día del vencimiento.
func BodyDueToday(ch repository.Charge) string {
	m := formatMoneyCL(ch.Amount)
	return fmt.Sprintf(`Hola 👋

Hoy vence el siguiente cobro:

💰 Monto: $%s  

¿Nos confirmas si el pago está en proceso?

Quedamos atentos.
%s`, m, signature(ch.ClientName))
}

// BodyOverdueFirst: primera notificación de mora.
func BodyOverdueFirst(ch repository.Charge) string {
	m := formatMoneyCL(ch.Amount)
	f := formatDateES(ch.DueDate)
	return fmt.Sprintf(`Hola 👋

El siguiente cobro está vencido:

💰 Monto: $%s  
📅 Vencía el %s  

Agradeceríamos nos confirmes estado de pago o fecha estimada.

Quedamos atentos.
%s`, m, f, signature(ch.ClientName))
}

// BodyOverdueFollowUp: seguimiento sobre cobro ya vencido.
func BodyOverdueFollowUp(ch repository.Charge) string {
	m := formatMoneyCL(ch.Amount)
	return fmt.Sprintf(`Hola 👋

Seguimos con este cobro pendiente:

💰 Monto: $%s  

Necesitamos confirmar fecha de pago para poder coordinar internamente.

Quedamos atentos a tu respuesta.
%s`, m, signature(ch.ClientName))
}
