package notify

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// TwilioConfig WhatsApp saliente (API oficial). El "From" es un numero de Twilio o sandbox.
type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	WhatsFrom  string // ej. whatsapp:+14155238886 (sandbox) o tu numero aprobado
}

func (c *TwilioConfig) enabled() bool {
	return c != nil && c.AccountSID != "" && c.AuthToken != "" && c.WhatsFrom != ""
}

func (d *Dispatcher) sendWhatsApp(toWhatsApp, body string, mediaURLs []string) {
	if toWhatsApp == "" {
		log.Printf("[FlowPay WhatsApp] sin numero destino; no se envia")
		return
	}
	if d.twilio == nil || !d.twilio.enabled() {
		log.Printf("[FlowPay WhatsApp mock -> %s] %s", toWhatsApp, strings.ReplaceAll(body, "\n", " "))
		if len(mediaURLs) > 0 {
			log.Printf("[FlowPay WhatsApp mock] adjuntos: %v", mediaURLs)
		}
		return
	}
	api := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", d.twilio.AccountSID)
	data := url.Values{}
	data.Set("From", d.twilio.WhatsFrom)
	data.Set("To", toWhatsApp)
	data.Set("Body", body)
	for _, u := range mediaURLs {
		if u != "" {
			data.Add("MediaUrl", u)
		}
	}
	req, err := http.NewRequest(http.MethodPost, api, strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("[FlowPay WhatsApp] req: %v", err)
		return
	}
	req.SetBasicAuth(d.twilio.AccountSID, d.twilio.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[FlowPay WhatsApp] error: %v", err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("[FlowPay WhatsApp] HTTP %d: %s", resp.StatusCode, string(b))
		return
	}
	log.Printf("[FlowPay WhatsApp] enviado a %s", toWhatsApp)
}
