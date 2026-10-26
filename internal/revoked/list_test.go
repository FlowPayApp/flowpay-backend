package revoked

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHasDropsExpired(t *testing.T) {
	list := New()
	list.Replace(map[string]time.Time{
		"vivo":    time.Now().Add(time.Minute),
		"vencido": time.Now().Add(-time.Second),
	})
	if !list.Has("vivo") {
		t.Fatal("el jti vigente tiene que estar revocado")
	}
	if list.Has("vencido") || list.Has("") || list.Has("otro") {
		t.Fatal("solo el jti vigente y no vencido cuenta")
	}
}

func TestWatchKeepsListWhenRefreshFails(t *testing.T) {
	previous := refreshEvery
	refreshEvery = 15 * time.Millisecond
	t.Cleanup(func() { refreshEvery = previous })

	list := New()
	var calls atomic.Int32
	load := func(context.Context) (map[string]time.Time, error) {
		n := calls.Add(1)
		if n == 1 {
			return map[string]time.Time{"vivo": time.Now().Add(time.Minute)}, nil
		}
		return nil, errors.New("base caída")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Watch(ctx, list, load); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatal("la renovación no llegó a ejecutarse")
	}
	if !list.Has("vivo") {
		t.Fatal("un fallo de renovación no puede vaciar la lista")
	}
}

func TestWatchReturnsFirstError(t *testing.T) {
	list := New()
	err := Watch(context.Background(), list, func(context.Context) (map[string]time.Time, error) {
		return nil, errors.New("sin tabla")
	})
	if err == nil {
		t.Fatal("la primera carga fallida tiene que volver error")
	}
	if list.Has("vivo") {
		t.Fatal("un fallo inicial no deja la lista marcada")
	}
}
