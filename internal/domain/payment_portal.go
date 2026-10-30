package domain

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

const maxPaymentPortalURL = 500

// NormalizePaymentPortalURL acepta solo un portal https público, sin acortadores ni datos del cobro.
func NormalizePaymentPortalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Pega la dirección del portal.")
	}
	if strings.Contains(raw, "{{") || strings.Contains(raw, "}}") {
		return "", errors.New("Pega solo la dirección del portal, sin datos del cobro.")
	}
	if len(raw) > maxPaymentPortalURL {
		return "", errors.New("La dirección es demasiado larga.")
	}
	if strings.ContainsAny(raw, " \t\r\n\\") {
		return "", errors.New("Esa dirección no es válida.")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", errors.New("Esa dirección no es válida.")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", errors.New("El portal tiene que usar https.")
	}
	if u.Host == "" || u.Opaque != "" {
		return "", errors.New("Esa dirección no es válida.")
	}
	if u.User != nil {
		return "", errors.New("La dirección no puede incluir usuario ni contraseña.")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return "", errors.New("El portal tiene que ser un dominio público.")
	}
	if net.ParseIP(host) != nil || strings.Contains(host, ":") {
		return "", errors.New("El portal tiene que ser un dominio, no una IP.")
	}
	if !strings.Contains(host, ".") {
		return "", errors.New("El portal tiene que ser un dominio público.")
	}
	bare := strings.TrimPrefix(host, "www.")
	if paymentPortalShortener(bare) {
		return "", errors.New("Usa la dirección completa del portal, sin acortadores.")
	}
	port := u.Port()
	if port != "" && port != "443" {
		u.Host = host + ":" + port
	} else {
		u.Host = host
	}
	u.Scheme = "https"
	u.Fragment = ""
	return u.String(), nil
}

func paymentPortalShortener(host string) bool {
	switch host {
	case "bit.ly", "bitly.com", "tinyurl.com", "t.co", "goo.gl", "ow.ly", "is.gd", "buff.ly",
		"rebrand.ly", "cutt.ly", "shorturl.at", "tiny.cc", "rb.gy", "s.id", "acortar.link", "lnkd.in":
		return true
	default:
		return false
	}
}
