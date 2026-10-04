package schedule

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	minutesPerDay = 24 * 60
	example       = `(example: "Mon-Fri 08:00-16:00")`
	dayHint       = "(use Mon, Tue, … or weekdays/weekends/daily)"
	windowHint    = "(use HH:MM-HH:MM, e.g. 08:00-16:00)"
	timeHint      = "(use 24-hour HH:MM, 00:00-24:00)"
)

// window is one time window, in minutes after midnight of the day it starts
// on. end > start; an end past minutesPerDay spans midnight.
type window struct{ start, end int }

var dayNames = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday,
}

var dayAliases = map[string][]time.Weekday{
	"daily":    {time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday},
	"weekdays": {time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
	"weekends": {time.Saturday, time.Sunday},
}

// Parse parses a schedule spec; see the package documentation for the
// grammar. Errors are meant for the user and name the offending token.
func Parse(spec string) (*Schedule, error) {
	s := &Schedule{}
	rules := 0
	for _, rule := range strings.Split(spec, ";") {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		days, windows, err := parseRule(rule)
		if err != nil {
			return nil, err
		}
		for _, d := range days {
			s.days[d] = append(s.days[d], windows...)
		}
		rules++
	}
	if rules == 0 {
		return nil, errors.New("schedule: empty schedule " + example)
	}
	for d := range s.days {
		slices.SortFunc(s.days[d], func(a, b window) int {
			if a.start != b.start {
				return a.start - b.start
			}
			return a.end - b.end
		})
		s.days[d] = slices.Compact(s.days[d])
	}
	s.always = coversWeek(&s.days)
	return s, nil
}

// parseRule splits "<days> <windows>" at the first digit.
func parseRule(rule string) ([]time.Weekday, []window, error) {
	i := strings.IndexAny(rule, "0123456789")
	if i < 0 {
		return nil, nil, fmt.Errorf("schedule: %q has no time window %s", rule, example)
	}
	daysPart := strings.TrimSpace(rule[:i])
	if daysPart == "" {
		return nil, nil, fmt.Errorf("schedule: %q has no days %s", rule, example)
	}
	days, err := parseDays(daysPart)
	if err != nil {
		return nil, nil, err
	}
	windows, err := parseWindows(strings.TrimSpace(rule[i:]))
	if err != nil {
		return nil, nil, err
	}
	return days, windows, nil
}

func parseDays(part string) ([]time.Weekday, error) {
	var set [7]bool
	for _, item := range strings.Split(part, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("schedule: empty day in %q %s", part, dayHint)
		}
		if lo, hi, isRange := strings.Cut(item, "-"); isRange {
			lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
			if lo == "" || hi == "" || strings.Contains(hi, "-") {
				return nil, fmt.Errorf("schedule: invalid day range %q (use a range like Mon-Fri)", item)
			}
			from, err := parseDay(lo, item)
			if err != nil {
				return nil, err
			}
			to, err := parseDay(hi, item)
			if err != nil {
				return nil, err
			}
			for d := from; ; d = (d + 1) % 7 {
				set[d] = true
				if d == to {
					break
				}
			}
			continue
		}
		if alias, ok := dayAliases[strings.ToLower(item)]; ok {
			for _, d := range alias {
				set[d] = true
			}
			continue
		}
		d, err := parseDay(item, "")
		if err != nil {
			return nil, err
		}
		set[d] = true
	}
	var days []time.Weekday
	for d, on := range set {
		if on {
			days = append(days, time.Weekday(d))
		}
	}
	return days, nil
}

// parseDay parses one day name; inRange is the range it is an end of, if any.
func parseDay(name, inRange string) (time.Weekday, error) {
	if d, ok := dayNames[strings.ToLower(name)]; ok {
		return d, nil
	}
	if _, ok := dayAliases[strings.ToLower(name)]; ok && inRange != "" {
		return 0, fmt.Errorf("schedule: %q cannot be part of a day range (%q)", name, inRange)
	}
	return 0, fmt.Errorf("schedule: unknown day %q %s", name, dayHint)
}

func parseWindows(part string) ([]window, error) {
	var windows []window
	for _, item := range strings.Split(part, ",") {
		raw := strings.TrimSpace(item)
		if raw == "" {
			return nil, fmt.Errorf("schedule: empty time window in %q %s", part, windowHint)
		}
		compact := strings.Join(strings.Fields(raw), "")
		from, to, ok := strings.Cut(compact, "-")
		if !ok || strings.Contains(to, "-") || !strings.Contains(from, ":") || !strings.Contains(to, ":") {
			return nil, fmt.Errorf("schedule: invalid time window %q %s", raw, windowHint)
		}
		start, err := parseTime(from, raw)
		if err != nil {
			return nil, err
		}
		end, err := parseTime(to, raw)
		if err != nil {
			return nil, err
		}
		switch {
		case start == minutesPerDay:
			return nil, fmt.Errorf("schedule: 24:00 can only end a window, not start one (%q)", raw)
		case end == start:
			return nil, fmt.Errorf("schedule: zero-length window %q (use 00:00-24:00 for a whole day)", raw)
		case end < start:
			end += minutesPerDay // spans midnight
		}
		windows = append(windows, window{start, end})
	}
	return windows, nil
}

// parseTime parses H:MM or HH:MM (00:00-24:00) into minutes after midnight.
func parseTime(s, raw string) (int, error) {
	bad := fmt.Errorf("schedule: invalid time %q in %q %s", s, raw, timeHint)
	h, m, _ := strings.Cut(s, ":")
	if len(h) < 1 || len(h) > 2 || len(m) != 2 || !digits(h) || !digits(m) {
		return 0, bad
	}
	hour := atoi(h)
	minute := atoi(m)
	if minute > 59 || hour > 24 || hour == 24 && minute != 0 {
		return 0, bad
	}
	return hour*60 + minute, nil
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// coversWeek reports whether the windows cover every minute of the week in
// wall-clock terms, so the schedule never has a transition.
func coversWeek(days *[7][]window) bool {
	var covered [7 * minutesPerDay]bool
	for d, windows := range days {
		base := d * minutesPerDay
		for _, w := range windows {
			for m := base + w.start; m < base+w.end; m++ {
				covered[m%len(covered)] = true
			}
		}
	}
	for _, c := range covered {
		if !c {
			return false
		}
	}
	return true
}

// String returns the schedule in normalized form: canonical day names, days
// with the same windows grouped into one rule, windows sorted and
// de-duplicated. Parse(s.String()) gives back the same schedule.
func (s *Schedule) String() string {
	type group struct {
		windows []window
		days    [7]bool // Monday first
	}
	var groups []*group
	for i := range 7 {
		windows := s.days[mondayFirst(i)]
		if len(windows) == 0 {
			continue
		}
		var g *group
		for _, cand := range groups {
			if slices.Equal(cand.windows, windows) {
				g = cand
				break
			}
		}
		if g == nil {
			g = &group{windows: windows}
			groups = append(groups, g)
		}
		g.days[i] = true
	}
	rules := make([]string, 0, len(groups))
	for _, g := range groups {
		ws := make([]string, len(g.windows))
		for i, w := range g.windows {
			ws[i] = formatMinute(w.start) + "-" + formatMinute(w.end)
		}
		rules = append(rules, formatDays(g.days)+" "+strings.Join(ws, ","))
	}
	return strings.Join(rules, "; ")
}

// mondayFirst maps 0..6 to Monday..Sunday.
func mondayFirst(i int) time.Weekday { return time.Weekday((i + 1) % 7) }

// formatDays writes a Monday-first day set as runs: "Mon", "Mon,Tue",
// "Mon-Wed", and a run through Sunday into Monday as "Fri-Mon".
func formatDays(set [7]bool) string {
	type run struct{ from, n int }
	var runs []run
	all := true
	for i, on := range set {
		switch {
		case !on:
			all = false
		case len(runs) > 0 && runs[len(runs)-1].from+runs[len(runs)-1].n == i:
			runs[len(runs)-1].n++
		default:
			runs = append(runs, run{i, 1})
		}
	}
	if all {
		return "daily"
	}
	if last := len(runs) - 1; last > 0 && runs[0].from == 0 && runs[last].from+runs[last].n == 7 && runs[0].n+runs[last].n >= 3 {
		runs[last].n += runs[0].n
		runs = runs[1:]
	}
	parts := make([]string, 0, len(runs))
	for _, r := range runs {
		first := mondayFirst(r.from).String()[:3]
		lastDay := mondayFirst((r.from + r.n - 1) % 7).String()[:3]
		switch r.n {
		case 1:
			parts = append(parts, first)
		case 2:
			parts = append(parts, first+","+lastDay)
		default:
			parts = append(parts, first+"-"+lastDay)
		}
	}
	return strings.Join(parts, ",")
}

// formatMinute writes minutes after midnight as HH:MM; exactly one day is
// "24:00" and later minutes wrap to the next day's clock time.
func formatMinute(m int) string {
	if m != minutesPerDay {
		m %= minutesPerDay
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}
