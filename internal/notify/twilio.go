package notify

import (
	"encoding/json"
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
	Templates  TemplateSIDs
}

func (c *TwilioConfig) credentialsOK() bool {
	return c != nil && strings.TrimSpace(c.AccountSID) != "" && strings.TrimSpace(c.AuthToken) != ""
}

func (d *Dispatcher) sendWhatsApp(toWhatsApp, body string, mediaURLs []string) {
	_ = d.sendWhatsAppFrom("", toWhatsApp, body, mediaURLs)
}

// sendWhatsAppFrom envía desde el número de la empresa. Si from está vacío, usa FLOWPAY_TWILIO_WHATSAPP_FROM.
func (d *Dispatcher) sendWhatsAppFrom(from, toWhatsApp, body string, mediaURLs []string) error {
	if toWhatsApp == "" {
		log.Printf("[FlowPay WhatsApp] sin numero destino; no se envia")
		return nil
	}
	from = strings.TrimSpace(from)
	if from == "" && d.twilio != nil {
		from = strings.TrimSpace(d.twilio.WhatsFrom)
		if from != "" {
			log.Printf("[FlowPay WhatsApp] sin número de empresa; se usa FLOWPAY_TWILIO_WHATSAPP_FROM")
		}
	}
	if from != "" {
		from = NormalizeWhatsAppForTwilio(from)
	}
	if d.twilio == nil || !d.twilio.credentialsOK() {
		log.Printf("[FlowPay WhatsApp mock -> %s from=%s] %s", toWhatsApp, from, strings.ReplaceAll(body, "\n", " "))
		if len(mediaURLs) > 0 {
			log.Printf("[FlowPay WhatsApp mock] adjuntos: %v", mediaURLs)
		}
		return nil
	}
	if from == "" {
		return fmt.Errorf("la empresa no tiene un número de WhatsApp Business")
	}
	data := url.Values{}
	data.Set("From", from)
	data.Set("To", toWhatsApp)
	data.Set("Body", body)
	for _, u := range mediaURLs {
		if u != "" {
			data.Add("MediaUrl", u)
		}
	}
	return d.postTwilioMessage(data, from, toWhatsApp)
}

// sendWhatsAppContent envía una plantilla aprobada (ContentSid + variables). Sin Body.
func (d *Dispatcher) sendWhatsAppContent(from, toWhatsApp, contentSID string, variables map[string]string) error {
	if toWhatsApp == "" {
		log.Printf("[FlowPay WhatsApp] sin numero destino; no se envia")
		return nil
	}
	from, mock, err := d.prepareWhatsAppFrom(from)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(variables)
	if err != nil {
		return fmt.Errorf("no se pudieron armar las variables de la plantilla")
	}
	if mock {
		log.Printf("[FlowPay WhatsApp mock -> %s from=%s] plantilla %s %s", toWhatsApp, from, contentSID, string(raw))
		return nil
	}
	data := url.Values{}
	data.Set("From", from)
	data.Set("To", toWhatsApp)
	data.Set("ContentSid", contentSID)
	data.Set("ContentVariables", string(raw))
	if err := d.postTwilioMessage(data, from, toWhatsApp); err != nil {
		return err
	}
	log.Printf("[FlowPay WhatsApp] plantilla %s a %s desde %s", contentSID, toWhatsApp, from)
	return nil
}

func (d *Dispatcher) prepareWhatsAppFrom(from string) (resolved string, mock bool, err error) {
	from = strings.TrimSpace(from)
	if from == "" && d.twilio != nil {
		from = strings.TrimSpace(d.twilio.WhatsFrom)
		if from != "" {
			log.Printf("[FlowPay WhatsApp] sin número de empresa; se usa FLOWPAY_TWILIO_WHATSAPP_FROM")
		}
	}
	if from != "" {
		from = NormalizeWhatsAppForTwilio(from)
	}
	if d.twilio == nil || !d.twilio.credentialsOK() {
		return from, true, nil
	}
	if from == "" {
		return "", false, fmt.Errorf("la empresa no tiene un número de WhatsApp Business")
	}
	return from, false, nil
}

func (d *Dispatcher) postTwilioMessage(data url.Values, from, to string) error {
	api := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", d.twilio.AccountSID)
	req, err := http.NewRequest(http.MethodPost, api, strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("[FlowPay WhatsApp] req: %v", err)
		return fmt.Errorf("no se pudo armar el envío de WhatsApp")
	}
	req.SetBasicAuth(d.twilio.AccountSID, d.twilio.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[FlowPay WhatsApp] error: %v", err)
		return fmt.Errorf("no se pudo contactar a Twilio")
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		msg := string(b)
		if len(msg) > 400 {
			msg = msg[:400]
		}
		log.Printf("[FlowPay WhatsApp] HTTP %d from=%s to=%s: %s", resp.StatusCode, from, to, msg)
		return fmt.Errorf("Twilio no envió el WhatsApp (HTTP %d): %s", resp.StatusCode, msg)
	}
	log.Printf("[FlowPay WhatsApp] enviado a %s desde %s", to, from)
	return nil
}
