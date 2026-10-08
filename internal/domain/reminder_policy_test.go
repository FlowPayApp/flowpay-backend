package domain

import (
	"reflect"
	"testing"
)

func TestNormalizedSortsAndDedupesDays(t *testing.T) {
	p, err := ReminderPolicy{DaysBefore: []int{0, 3, 1, 3}, OverdueEvery: 2, OverdueMax: OverdueUnlimited}.Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{3, 1, 0}; !reflect.DeepEqual(p.DaysBefore, want) {
		t.Fatalf("días = %v, se esperaba %v", p.DaysBefore, want)
	}
}

func TestNormalizedRejectsOutOfRange(t *testing.T) {
	bad := []ReminderPolicy{
		{DaysBefore: []int{-1}, OverdueEvery: 1},
		{DaysBefore: []int{61}, OverdueEvery: 1},
		{OverdueEvery: 0},
		{OverdueEvery: 1, OverdueMax: -2},
		{OverdueEvery: 1, OverdueMax: 101},
	}
	for _, p := range bad {
		if _, err := p.Normalized(); err == nil {
			t.Errorf("%+v debería ser inválida", p)
		}
	}
}

func TestReminderDaysRoundTrip(t *testing.T) {
	days := []int{7, 3, 0}
	if got := ParseReminderDays(FormatReminderDays(days)); !reflect.DeepEqual(got, days) {
		t.Fatalf("got %v", got)
	}
	if got := ParseReminderDays(""); len(got) != 0 {
		t.Fatalf("vacío debería no tener días, got %v", got)
	}
}
