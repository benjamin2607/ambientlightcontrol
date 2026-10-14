package automation

import (
	"testing"
	"time"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 14, hour, minute, 0, 0, time.UTC)
}

func mustSchedule(t *testing.T, on, off string) Schedule {
	t.Helper()
	a, err := ParseClock(on)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseClock(off)
	if err != nil {
		t.Fatal(err)
	}
	return Schedule{On: a, Off: b}
}

func TestParseClock(t *testing.T) {
	c, err := ParseClock("07:05")
	if err != nil || c != 425 || c.String() != "07:05" {
		t.Fatalf("got %v %v", c, err)
	}
	for _, bad := range []string{"", "7", "24:00", "12:60", "ab:cd"} {
		if _, err := ParseClock(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestScheduleContains(t *testing.T) {
	day := mustSchedule(t, "07:00", "22:00")
	cases := []struct {
		s    Schedule
		h, m int
		want bool
	}{
		{day, 6, 59, false},
		{day, 7, 0, true},
		{day, 21, 59, true},
		{day, 22, 0, false},
		{day, 0, 0, false},
		{mustSchedule(t, "20:00", "02:00"), 23, 0, true},
		{mustSchedule(t, "20:00", "02:00"), 1, 59, true},
		{mustSchedule(t, "20:00", "02:00"), 2, 0, false},
		{mustSchedule(t, "20:00", "02:00"), 12, 0, false},
		{mustSchedule(t, "08:00", "08:00"), 8, 0, false},
	}
	for _, c := range cases {
		if got := c.s.Contains(at(c.h, c.m)); got != c.want {
			t.Errorf("%v-%v at %02d:%02d = %v, want %v", c.s.On, c.s.Off, c.h, c.m, got, c.want)
		}
	}
}

func TestNextChange(t *testing.T) {
	s := mustSchedule(t, "07:00", "22:00")
	if got := s.NextChange(at(6, 0)); !got.Equal(at(7, 0)) {
		t.Errorf("got %v", got)
	}
	if got := s.NextChange(at(7, 0)); !got.Equal(at(22, 0)) {
		t.Errorf("got %v", got)
	}
	if got := s.NextChange(at(23, 0)); !got.Equal(at(7, 0).AddDate(0, 0, 1)) {
		t.Errorf("got %v", got)
	}
	if !mustSchedule(t, "08:00", "08:00").NextChange(at(1, 0)).IsZero() {
		t.Error("empty window should have no change")
	}
}

func TestDecide(t *testing.T) {
	cases := []struct {
		ctx  Context
		want Target
	}{
		{Context{InWindow: false, TVActive: false}, TargetOff},
		{Context{InWindow: true, TVActive: false}, TargetColor},
		{Context{InWindow: true, TVActive: true}, TargetVideo},
		// After 22:00 the light stays on while the TV runs.
		{Context{InWindow: false, TVActive: true}, TargetVideo},
	}
	for _, c := range cases {
		if got := Decide(c.ctx); got != c.want {
			t.Errorf("%+v = %v, want %v", c.ctx, got, c.want)
		}
	}
}
