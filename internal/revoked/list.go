package revoked

import (
	"context"
	"log"
	"sync"
	"time"
)

// List es la copia en memoria de revoked_access_tokens. Validar un token no consulta la base.
type List struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func New() *List {
	return &List{until: map[string]time.Time{}}
}

func (l *List) Replace(items map[string]time.Time) {
	if items == nil {
		items = map[string]time.Time{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.until = items
}

func (l *List) Has(jti string) bool {
	if jti == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	exp, ok := l.until[jti]
	if !ok {
		return false
	}
	if !exp.After(time.Now()) {
		delete(l.until, jti)
		return false
	}
	return true
}

// refreshEvery es el tiempo máximo que un cierre de sesión tarda en llegar a este proceso.
var refreshEvery = 5 * time.Second

// Watch carga la lista y la renueva. Si una renovación falla, se conserva la copia anterior.
func Watch(ctx context.Context, list *List, load func(context.Context) (map[string]time.Time, error)) error {
	first, cancel := context.WithTimeout(ctx, 5*time.Second)
	items, err := load(first)
	cancel()
	if err != nil {
		return err
	}
	list.Replace(items)
	go func() {
		ticker := time.NewTicker(refreshEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next, err := load(ctx)
				if err != nil {
					log.Printf("warn: jti revocados: %v", err)
					continue
				}
				list.Replace(next)
			}
		}
	}()
	return nil
}
