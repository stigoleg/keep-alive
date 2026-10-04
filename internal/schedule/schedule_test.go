package schedule

import (
	"testing"
	"time"
	_ "time/tzdata" // Europe/Oslo even where the system has no tz database
)

// 2026-10-05 is a Monday.
func utc(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
}

func mustParse(t *testing.T, spec string) *Schedule {
	t.Helper()
	s, err := Parse(spec)
	if err != nil {
		t.Fatalf("Parse(%q): %v", spec, err)
	}
	return s
}

func TestIn(t *testing.T) {
	tests := []struct {
		spec string
		at   time.Time
		want bool
	}{
		{"Mon-Fri 08:00-16:00", utc(5, 7, 59), false},
		{"Mon-Fri 08:00-16:00", utc(5, 8, 0), true},
		{"Mon-Fri 08:00-16:00", utc(5, 15, 59), true},
		{"Mon-Fri 08:00-16:00", utc(5, 16, 0), false}, // windows are half-open
		{"Mon-Fri 08:00-16:00", utc(9, 12, 0), true},  // Friday
		{"Mon-Fri 08:00-16:00", utc(10, 12, 0), false},
		{"Mon-Fri 08:00-16:00", utc(11, 12, 0), false},
		// A midnight-spanning window belongs to its start day.
		{"Fri 22:00-02:00", utc(9, 23, 0), true},
		{"Fri 22:00-02:00", utc(10, 1, 59), true},
		{"Fri 22:00-02:00", utc(10, 2, 0), false},
		{"Fri 22:00-02:00", utc(9, 1, 0), false}, // Thursday night is not in it
		{"Fri 22:00-02:00", utc(8, 23, 0), false},
		// Across the week boundary.
		{"Sun 22:00-02:00", utc(11, 23, 0), true},
		{"Sun 22:00-02:00", utc(12, 1, 0), true},
		{"Sun 22:00-02:00", utc(5, 1, 0), true}, // last Sunday's window
		{"Sun 22:00-02:00", utc(5, 2, 0), false},
		{"daily 22:00-06:00", utc(7, 3, 0), true},
		{"daily 22:00-06:00", utc(7, 12, 0), false},
		{"Fri-Mon 09:00-17:00", utc(11, 12, 0), true},
		{"Fri-Mon 09:00-17:00", utc(12, 12, 0), true},
		{"Fri-Mon 09:00-17:00", utc(13, 12, 0), false},
		{"Mon 22:00-24:00", utc(5, 23, 59), true},
		{"Mon 22:00-24:00", utc(6, 0, 0), false},
		{"daily 00:00-24:00", utc(8, 3, 17), true},
		{"weekdays 08:00-11:30,12:00-16:00; Sat 10:00-12:00", utc(6, 11, 45), false},
		{"weekdays 08:00-11:30,12:00-16:00; Sat 10:00-12:00", utc(10, 11, 0), true},
	}
	for _, tt := range tests {
		s := mustParse(t, tt.spec)
		if got := s.In(tt.at); got != tt.want {
			t.Errorf("%q.In(%s) = %v, want %v", tt.spec, tt.at.Format("Mon 2006-01-02 15:04"), got, tt.want)
		}
	}
}

// TestInUsesTimeLocation checks that a schedule is read in the location of the
// time it is asked about.
func TestInUsesTimeLocation(t *testing.T) {
	s := mustParse(t, "Mon-Fri 08:00-16:00")
	plus3 := time.FixedZone("UTC+3", 3*3600)
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, plus3) // 06:00 UTC
	if !s.In(at) {
		t.Fatal("09:00 local is outside 08:00-16:00")
	}
	if s.In(at.UTC()) {
		t.Fatal("06:00 UTC is inside 08:00-16:00")
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		spec     string
		at       time.Time
		want     time.Time
		entering bool
	}{
		{"Mon-Fri 08:00-16:00", utc(5, 7, 0), utc(5, 8, 0), true},
		{"Mon-Fri 08:00-16:00", utc(5, 8, 0), utc(5, 16, 0), false}, // strictly after t
		{"Mon-Fri 08:00-16:00", utc(5, 16, 0), utc(6, 8, 0), true},
		{"Mon-Fri 08:00-16:00", utc(9, 16, 0), utc(12, 8, 0), true}, // over the weekend
		{"Mon-Fri 08:00-16:00", utc(10, 12, 0), utc(12, 8, 0), true},
		{"Sun 10:00-11:00", utc(11, 11, 0), utc(18, 10, 0), true}, // a whole week ahead
		// Adjacent and overlapping windows are one window.
		{"Mon 08:00-12:00,12:00-16:00", utc(5, 9, 0), utc(5, 16, 0), false},
		{"Mon 08:00-12:00,11:00-16:00", utc(5, 9, 0), utc(5, 16, 0), false},
		{"Mon 08:00-12:00,09:00-10:00", utc(5, 9, 30), utc(5, 12, 0), false},
		{"Mon 22:00-24:00; Tue 00:00-02:00", utc(5, 23, 0), utc(6, 2, 0), false},
		{"Mon 22:00-02:00; Tue 01:00-03:00", utc(5, 23, 0), utc(6, 3, 0), false},
		{"daily 22:00-06:00", utc(5, 23, 0), utc(6, 6, 0), false},
		{"daily 22:00-06:00", utc(6, 6, 0), utc(6, 22, 0), true},
		{"Mon-Sat 00:00-24:00; Sun 00:00-23:00", utc(5, 10, 0), utc(11, 23, 0), false},
		{"Mon-Sat 00:00-24:00; Sun 00:00-23:00", utc(11, 23, 0), utc(12, 0, 0), true},
	}
	for _, tt := range tests {
		s := mustParse(t, tt.spec)
		got, entering := s.Next(tt.at)
		if !got.Equal(tt.want) || entering != tt.entering {
			t.Errorf("%q.Next(%s) = %s, %v; want %s, %v", tt.spec, tt.at.Format("Mon 01-02 15:04"),
				got.Format("Mon 01-02 15:04"), entering, tt.want.Format("Mon 01-02 15:04"), tt.entering)
		}
	}
}

func TestCurrent(t *testing.T) {
	tests := []struct {
		spec       string
		at         time.Time
		start, end time.Time
		ok         bool
	}{
		{"Mon-Fri 08:00-16:00", utc(5, 7, 0), time.Time{}, time.Time{}, false},
		{"Mon-Fri 08:00-16:00", utc(5, 8, 0), utc(5, 8, 0), utc(5, 16, 0), true},
		{"Mon-Fri 08:00-16:00", utc(5, 16, 0), time.Time{}, time.Time{}, false},
		{"Fri 22:00-02:00", utc(10, 1, 0), utc(9, 22, 0), utc(10, 2, 0), true},
		{"Mon 08:00-12:00,12:00-16:00", utc(5, 13, 0), utc(5, 8, 0), utc(5, 16, 0), true},
		{"Mon 08:00-12:00,11:00-16:00,07:00-08:30", utc(5, 13, 0), utc(5, 7, 0), utc(5, 16, 0), true},
		{"Mon 22:00-24:00; Tue 00:00-02:00", utc(6, 1, 0), utc(5, 22, 0), utc(6, 2, 0), true},
		{"daily 22:00-06:00; Tue 06:00-08:00", utc(6, 7, 0), utc(5, 22, 0), utc(6, 8, 0), true},
		{"Mon-Sat 00:00-24:00; Sun 00:00-23:00", utc(8, 10, 0), utc(5, 0, 0), utc(11, 23, 0), true},
	}
	for _, tt := range tests {
		s := mustParse(t, tt.spec)
		start, end, ok := s.Current(tt.at)
		if ok != tt.ok || !start.Equal(tt.start) || !end.Equal(tt.end) {
			t.Errorf("%q.Current(%s) = %s, %s, %v; want %s, %s, %v", tt.spec, tt.at.Format("Mon 01-02 15:04"),
				start.Format("Mon 01-02 15:04"), end.Format("Mon 01-02 15:04"), ok,
				tt.start.Format("Mon 01-02 15:04"), tt.end.Format("Mon 01-02 15:04"), tt.ok)
		}
	}
}

func TestAlwaysOn(t *testing.T) {
	for _, spec := range []string{
		"daily 00:00-24:00",
		"daily 12:00-12:30,12:30-12:00",
		"Mon-Sat 00:00-24:00; Sun 00:00-23:00,23:00-01:00",
	} {
		s := mustParse(t, spec)
		at := utc(7, 12, 34)
		if !s.In(at) {
			t.Errorf("%q.In = false", spec)
		}
		if next, entering := s.Next(at); !next.IsZero() || entering {
			t.Errorf("%q.Next = %v, %v; want zero time (no transition)", spec, next, entering)
		}
		if start, end, ok := s.Current(at); !ok || !start.IsZero() || !end.IsZero() {
			t.Errorf("%q.Current = %v, %v, %v; want zero bounds and ok", spec, start, end, ok)
		}
	}
}

func oslo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		t.Skipf("no tz database: %v", err)
	}
	return loc
}

func utcAt(y int, m time.Month, d, hour, minute int) time.Time {
	return time.Date(y, m, d, hour, minute, 0, 0, time.UTC)
}

// TestSpringForward covers 2026-03-29 in Oslo: 02:00 CET jumps to 03:00 CEST
// (01:00 UTC), so the day has 23 hours and 02:00-02:59 does not exist.
func TestSpringForward(t *testing.T) {
	loc := oslo(t)

	// Saturday evening to Sunday morning: 22:00 CET to 06:00 CEST is 7 hours.
	s := mustParse(t, "Sat 22:00-06:00")
	start, end, ok := s.Current(utcAt(2026, 3, 29, 3, 0).In(loc))
	if !ok || !start.Equal(utcAt(2026, 3, 28, 21, 0)) || !end.Equal(utcAt(2026, 3, 29, 4, 0)) {
		t.Fatalf("Sat 22:00-06:00 = %v - %v, %v", start, end, ok)
	}

	// The next 08:00 after Saturday 16:00 is Sunday 08:00 CEST (06:00 UTC),
	// not Saturday 16:00 + 16h (07:00 UTC).
	s = mustParse(t, "daily 08:00-16:00")
	next, entering := s.Next(utcAt(2026, 3, 28, 15, 0).In(loc))
	if !entering || !next.Equal(utcAt(2026, 3, 29, 6, 0)) {
		t.Fatalf("daily Next across spring-forward = %v, %v; want 06:00 UTC", next, entering)
	}

	// A boundary inside the gap is moved forward by the gap: 02:30 becomes
	// 03:30 CEST (01:30 UTC).
	s = mustParse(t, "Sun 02:30-04:00")
	next, entering = s.Next(time.Date(2026, 3, 29, 0, 0, 0, 0, loc))
	if !entering || !next.Equal(utcAt(2026, 3, 29, 1, 30)) {
		t.Fatalf("Sun 02:30 on spring-forward day = %v, %v; want 01:30 UTC", next, entering)
	}
	if s.In(utcAt(2026, 3, 29, 1, 15).In(loc)) { // 03:15 CEST
		t.Fatal("03:15 CEST is inside a window that starts at the shifted 03:30")
	}
	if !s.In(utcAt(2026, 3, 29, 1, 45).In(loc)) { // 03:45 CEST
		t.Fatal("03:45 CEST is outside Sun 02:30-04:00")
	}

	// A window entirely inside the gap keeps its length when both ends shift.
	s = mustParse(t, "Sun 02:15-02:45")
	start, end, ok = s.Current(utcAt(2026, 3, 29, 1, 30).In(loc))
	if !ok || !start.Equal(utcAt(2026, 3, 29, 1, 15)) || !end.Equal(utcAt(2026, 3, 29, 1, 45)) {
		t.Fatalf("Sun 02:15-02:45 on spring-forward day = %v - %v, %v", start, end, ok)
	}

	// A window whose shifted start lands after its end does not happen that
	// day: 02:30 becomes 03:30 CEST, after the 03:00 end.
	s = mustParse(t, "Sun 02:30-03:00")
	next, entering = s.Next(time.Date(2026, 3, 29, 0, 0, 0, 0, loc))
	if want := time.Date(2026, 4, 5, 2, 30, 0, 0, loc); !entering || !next.Equal(want) {
		t.Fatalf("Sun 02:30-03:00 Next = %v, %v; want the following Sunday %v", next, entering, want)
	}
	for m := 0; m < 24*60; m += 5 {
		at := time.Date(2026, 3, 29, 0, 0, 0, 0, loc).Add(time.Duration(m) * time.Minute)
		if s.In(at) {
			t.Fatalf("Sun 02:30-03:00 is active at %v on the spring-forward day", at)
		}
	}
}

// TestFallBack covers 2026-10-25 in Oslo: 03:00 CEST goes back to 02:00 CET
// (01:00 UTC), so the day has 25 hours and 02:00-02:59 happens twice. Go's
// time.Date resolves a repeated time in Oslo to the second (CET) occurrence.
func TestFallBack(t *testing.T) {
	loc := oslo(t)

	// Saturday 22:00 CEST to Sunday 06:00 CET is 9 hours.
	s := mustParse(t, "Sat 22:00-06:00")
	start, end, ok := s.Current(utcAt(2026, 10, 25, 0, 30).In(loc))
	if !ok || !start.Equal(utcAt(2026, 10, 24, 20, 0)) || !end.Equal(utcAt(2026, 10, 25, 5, 0)) {
		t.Fatalf("Sat 22:00-06:00 = %v - %v, %v", start, end, ok)
	}
	if got := end.Sub(start); got != 9*time.Hour {
		t.Fatalf("window length = %v, want 9h", got)
	}

	s = mustParse(t, "daily 08:00-16:00")
	next, entering := s.Next(utcAt(2026, 10, 24, 14, 0).In(loc))
	if !entering || !next.Equal(utcAt(2026, 10, 25, 7, 0)) {
		t.Fatalf("daily Next across fall-back = %v, %v; want 07:00 UTC", next, entering)
	}

	// 02:30 is the second occurrence, 02:30 CET (01:30 UTC).
	s = mustParse(t, "Sun 02:30-04:00")
	if s.In(utcAt(2026, 10, 25, 0, 45).In(loc)) { // 02:45 CEST, first occurrence
		t.Fatal("the first 02:45 (CEST) is inside a window that starts at 02:30 CET")
	}
	if !s.In(utcAt(2026, 10, 25, 1, 45).In(loc)) { // 02:45 CET, second occurrence
		t.Fatal("the second 02:45 (CET) is outside Sun 02:30-04:00")
	}
	next, entering = s.Next(time.Date(2026, 10, 25, 0, 0, 0, 0, loc))
	if !entering || !next.Equal(utcAt(2026, 10, 25, 1, 30)) {
		t.Fatalf("Sun 02:30 on fall-back day = %v, %v; want 01:30 UTC", next, entering)
	}
}

// TestConsistency sweeps two weeks around each Oslo daylight-saving change and
// checks that In, Current and Next agree with each other.
func TestConsistency(t *testing.T) {
	loc := oslo(t)
	specs := []string{
		"Mon-Fri 08:00-16:00",
		"weekdays 08:00-11:30,12:00-16:00; Sat 10:00-12:00",
		"daily 22:00-06:00",
		"Fri-Mon 09:00-17:00",
		"Sun 01:30-02:30,02:30-03:30; Sat 23:00-02:15",
		"Mon-Sat 00:00-24:00; Sun 00:00-02:30",
	}
	starts := []time.Time{
		time.Date(2026, 3, 23, 0, 0, 0, 0, loc),
		time.Date(2026, 10, 19, 0, 0, 0, 0, loc),
	}
	for _, spec := range specs {
		s := mustParse(t, spec)
		for _, from := range starts {
			for at := from; at.Before(from.Add(14 * 24 * time.Hour)); at = at.Add(5 * time.Minute) {
				in := s.In(at)
				start, end, ok := s.Current(at)
				if in != ok {
					t.Fatalf("%q at %v: In = %v, Current ok = %v", spec, at, in, ok)
				}
				next, entering := s.Next(at)
				if !next.After(at) || entering == in {
					t.Fatalf("%q at %v: Next = %v, %v (in = %v)", spec, at, next, entering, in)
				}
				if s.In(next) != entering || s.In(next.Add(-time.Nanosecond)) != in {
					t.Fatalf("%q at %v: Next = %v is not a transition", spec, at, next)
				}
				if ok {
					if !next.Equal(end) || at.Before(start) || !at.Before(end) || s.In(start.Add(-time.Nanosecond)) {
						t.Fatalf("%q at %v: Current = %v - %v, Next = %v", spec, at, start, end, next)
					}
				}
			}
		}
	}
}
