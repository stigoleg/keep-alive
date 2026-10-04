package schedule

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		spec string
		want string // String() of the parsed schedule
	}{
		{"Mon-Fri 08:00-16:00", "Mon-Fri 08:00-16:00"},
		{"weekdays 08:00-11:30,12:00-16:00; Sat 10:00-12:00", "Mon-Fri 08:00-11:30,12:00-16:00; Sat 10:00-12:00"},
		{"daily 22:00-06:00", "daily 22:00-06:00"},
		{"Fri-Mon 09:00-17:00", "Fri-Mon 09:00-17:00"},
		{"Mon-Wed,Fri 08:00-16:00", "Mon-Wed,Fri 08:00-16:00"},
		{"Mon,Wed,Fri 08:00-16:00", "Mon,Wed,Fri 08:00-16:00"},
		{"  mOn , wed ,FRI   9:00 - 17:00 ", "Mon,Wed,Fri 09:00-17:00"},
		{"MONDAY-friday 08:00-16:00", "Mon-Fri 08:00-16:00"},
		{"Sunday 10:00-12:00", "Sun 10:00-12:00"},
		{"weekends 10:00-24:00", "Sat,Sun 10:00-24:00"},
		{"WEEKDAYS 08:00-16:00", "Mon-Fri 08:00-16:00"},
		{"Mon-Sun 00:00-24:00", "daily 00:00-24:00"},
		{"weekdays,weekends 08:00-09:00", "daily 08:00-09:00"},
		{"Sat,Sun,Mon 08:00-09:00", "Sat-Mon 08:00-09:00"},
		{"Mon,Sun 08:00-09:00", "Mon,Sun 08:00-09:00"},
		{"Mon,Wed,Fri,Sat,Sun 08:00-09:00", "Wed,Fri-Mon 08:00-09:00"},
		{"Fri-Fri 08:00-09:00", "Fri 08:00-09:00"},
		{"Mon 08:00-16:00; Tue 08:00-16:00", "Mon,Tue 08:00-16:00"},
		{"Mon 12:00-13:00,08:00-09:00,08:00-09:00", "Mon 08:00-09:00,12:00-13:00"},
		{"Mon-Fri 08:00-16:00; Mon 18:00-20:00", "Mon 08:00-16:00,18:00-20:00; Tue-Fri 08:00-16:00"},
		{"Mon 22:00-00:00", "Mon 22:00-24:00"},
		{"Mon 00:00-24:00", "Mon 00:00-24:00"},
		{"Mon 23:59-23:58", "Mon 23:59-23:58"},
		{"Mon 08:00-16:00;", "Mon 08:00-16:00"},
		{" ; Mon 08:00-16:00 ;; ", "Mon 08:00-16:00"},
		{"Mon-Fri08:00-16:00", "Mon-Fri 08:00-16:00"},
		{"Mon 08:00-11:30, 12:00-16:00", "Mon 08:00-11:30,12:00-16:00"},
		{"Tue 10:00-11:00; Mon 10:00-11:00; Sun 09:00-10:00", "Mon,Tue 10:00-11:00; Sun 09:00-10:00"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			s, err := Parse(tt.spec)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.spec, err)
			}
			got := s.String()
			if got != tt.want {
				t.Fatalf("Parse(%q).String() = %q, want %q", tt.spec, got, tt.want)
			}
			// The normalized form re-parses to itself and to the same schedule.
			again, err := Parse(got)
			if err != nil {
				t.Fatalf("re-parse of %q: %v", got, err)
			}
			if again.String() != got {
				t.Fatalf("re-parse of %q gives %q", got, again.String())
			}
			if !reflect.DeepEqual(again.days, s.days) {
				t.Fatalf("re-parse of %q changed the schedule", got)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		spec string
		want string // the exact error text
	}{
		{"", `schedule: empty schedule (example: "Mon-Fri 08:00-16:00")`},
		{"  ;  ; ", `schedule: empty schedule (example: "Mon-Fri 08:00-16:00")`},
		{"Mnday 08:00-16:00", `schedule: unknown day "Mnday" (use Mon, Tue, … or weekdays/weekends/daily)`},
		{"Mon-Frx 08:00-16:00", `schedule: unknown day "Frx" (use Mon, Tue, … or weekdays/weekends/daily)`},
		{"Mon Wed 08:00-16:00", `schedule: unknown day "Mon Wed" (use Mon, Tue, … or weekdays/weekends/daily)`},
		{"Mon,,Wed 08:00-16:00", `schedule: empty day in "Mon,,Wed" (use Mon, Tue, … or weekdays/weekends/daily)`},
		{"Mon-Tue-Wed 08:00-16:00", `schedule: invalid day range "Mon-Tue-Wed" (use a range like Mon-Fri)`},
		{"Mon- 08:00-16:00", `schedule: invalid day range "Mon-" (use a range like Mon-Fri)`},
		{"weekdays-Sun 08:00-16:00", `schedule: "weekdays" cannot be part of a day range ("weekdays-Sun")`},
		{"Mon", `schedule: "Mon" has no time window (example: "Mon-Fri 08:00-16:00")`},
		{"Mon-Fri; Sat 10:00-12:00", `schedule: "Mon-Fri" has no time window (example: "Mon-Fri 08:00-16:00")`},
		{"08:00-16:00", `schedule: "08:00-16:00" has no days (example: "Mon-Fri 08:00-16:00")`},
		{"Mon 8-16", `schedule: invalid time window "8-16" (use HH:MM-HH:MM, e.g. 08:00-16:00)`},
		{"Mon 08:00", `schedule: invalid time window "08:00" (use HH:MM-HH:MM, e.g. 08:00-16:00)`},
		{"Mon 08:00-12:00-16:00", `schedule: invalid time window "08:00-12:00-16:00" (use HH:MM-HH:MM, e.g. 08:00-16:00)`},
		{"Mon 08:00-16:00 Tue 09:00-10:00", `schedule: invalid time window "08:00-16:00 Tue 09:00-10:00" (use HH:MM-HH:MM, e.g. 08:00-16:00)`},
		{"Mon 08:00-16:00,", `schedule: empty time window in "08:00-16:00," (use HH:MM-HH:MM, e.g. 08:00-16:00)`},
		{"Mon 25:00-26:00", `schedule: invalid time "25:00" in "25:00-26:00" (use 24-hour HH:MM, 00:00-24:00)`},
		{"Mon 08:60-09:00", `schedule: invalid time "08:60" in "08:60-09:00" (use 24-hour HH:MM, 00:00-24:00)`},
		{"Mon 08:00-24:01", `schedule: invalid time "24:01" in "08:00-24:01" (use 24-hour HH:MM, 00:00-24:00)`},
		{"Mon 8:0-09:00", `schedule: invalid time "8:0" in "8:0-09:00" (use 24-hour HH:MM, 00:00-24:00)`},
		{"Mon 123:00-09:00", `schedule: invalid time "123:00" in "123:00-09:00" (use 24-hour HH:MM, 00:00-24:00)`},
		{"Mon 08:0a-09:00", `schedule: invalid time "08:0a" in "08:0a-09:00" (use 24-hour HH:MM, 00:00-24:00)`},
		{"Mon 24:00-02:00", `schedule: 24:00 can only end a window, not start one ("24:00-02:00")`},
		{"Mon 08:00-08:00", `schedule: zero-length window "08:00-08:00" (use 00:00-24:00 for a whole day)`},
		{"Mon 00:00-00:00", `schedule: zero-length window "00:00-00:00" (use 00:00-24:00 for a whole day)`},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			s, err := Parse(tt.spec)
			if err == nil {
				t.Fatalf("Parse(%q) = %q, want error %q", tt.spec, s, tt.want)
			}
			if err.Error() != tt.want {
				t.Fatalf("Parse(%q) error\n got: %s\nwant: %s", tt.spec, err, tt.want)
			}
		})
	}
}

// TestParseDayNames checks every accepted spelling of every day.
func TestParseDayNames(t *testing.T) {
	names := map[string]time.Weekday{
		"mon": time.Monday, "monday": time.Monday,
		"tue": time.Tuesday, "tuesday": time.Tuesday,
		"wed": time.Wednesday, "wednesday": time.Wednesday,
		"thu": time.Thursday, "thursday": time.Thursday,
		"fri": time.Friday, "friday": time.Friday,
		"sat": time.Saturday, "saturday": time.Saturday,
		"sun": time.Sunday, "sunday": time.Sunday,
	}
	for name, wd := range names {
		for _, spelled := range []string{name, strings.ToUpper(name), strings.ToUpper(name[:1]) + name[1:]} {
			s, err := Parse(spelled + " 08:00-09:00")
			if err != nil {
				t.Fatalf("Parse(%q): %v", spelled, err)
			}
			for d := range s.days {
				if got, want := len(s.days[d]) > 0, time.Weekday(d) == wd; got != want {
					t.Fatalf("Parse(%q): day %v set = %v, want %v", spelled, time.Weekday(d), got, want)
				}
			}
		}
	}
}
