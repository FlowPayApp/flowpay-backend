package notify

import (
	"log"
	"strings"
)

// sendSMS deja trazabilidad en logs; puede conectarse a proveedor SMS más adelante.
func (d *Dispatcher) sendSMS(toPhone, body string) {
	if toPhone == "" {
		log.Printf("[FlowPay SMS] sin número destino; no se envia")
		return
	}
	log.Printf("[FlowPay SMS mock -> %s] %s", toPhone, strings.ReplaceAll(body, "\n", " "))
}

