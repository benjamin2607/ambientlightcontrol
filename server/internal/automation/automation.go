// Package automation decides what the backlight should do. It is pure logic without I/O.
package automation

import (
	"fmt"
	"time"
)

// Target is the state the automation wants the light to be in.
type Target string

const (
	TargetOff   Target = "off"
	TargetColor Target = "color" // fixed daytime colour
	TargetVideo Target = "video" // follows the TV picture
)

// Clock is a time of day in minutes since midnight.
type Clock int

func ParseClock(s string) (Clock, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q, expected HH:MM", s)
	}
	return Clock(t.Hour()*60 + t.Minute()), nil
}

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60) }

// Schedule is the daily window in which the light shows the fixed colour. Off may be earlier than
// On, then the window runs past midnight. On == Off means the window is empty.
type Schedule struct {
	On, Off Clock
}

func (s Schedule) Contains(t time.Time) bool {
	m := Clock(t.Hour()*60 + t.Minute())
	switch {
	case s.On == s.Off:
		return false
	case s.On < s.Off:
		return m >= s.On && m < s.Off
	default:
		return m >= s.On || m < s.Off
	}
}

// NextChange returns the next time the window opens or closes, or zero time for an empty window.
func (s Schedule) NextChange(now time.Time) time.Time {
	if s.On == s.Off {
		return time.Time{}
	}
	var next time.Time
	for _, c := range []Clock{s.On, s.Off} {
		t := time.Date(now.Year(), now.Month(), now.Day(), int(c)/60, int(c)%60, 0, 0, now.Location())
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		if next.IsZero() || t.Before(next) {
			next = t
		}
	}
	return next
}

// Context is everything the decision depends on. A manual change lasts until the context changes.
type Context struct {
	InWindow bool
	TVActive bool
}

// Decide: the TV wins, otherwise the schedule.
func Decide(c Context) Target {
	switch {
	case c.TVActive:
		return TargetVideo
	case c.InWindow:
		return TargetColor
	default:
		return TargetOff
	}
}

// Reason is a short explanation for the UI.
func Reason(c Context, s Schedule) string {
	switch {
	case c.TVActive:
		return "TV is on"
	case c.InWindow:
		return fmt.Sprintf("Daytime %s–%s", s.On, s.Off)
	default:
		return fmt.Sprintf("Night, on again at %s", s.On)
	}
}
