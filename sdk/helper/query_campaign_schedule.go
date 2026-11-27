package sdkHelper

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var istLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}()

// IST returns Asia/Kolkata.
func IST() *time.Location { return istLocation }

// ParseSchedule unmarshals and validates ScheduleJson.
func ParseSchedule(raw json.RawMessage, queryTimeoutSec int) (Schedule, error) {
	var s Schedule
	if len(raw) == 0 {
		return s, fmt.Errorf("scheduleJson is required")
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("scheduleJson is invalid: %w", err)
	}
	if strings.TrimSpace(s.Timezone) == "" {
		s.Timezone = "Asia/Kolkata"
	}
	if !strings.EqualFold(s.Timezone, "Asia/Kolkata") {
		return s, fmt.Errorf("scheduleJson.timezone must be Asia/Kolkata")
	}
	hour, minute, err := parseHHMM(s.Time)
	if err != nil {
		return s, err
	}
	tod := time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute
	windowStart := 9 * time.Hour
	windowEnd := 20 * time.Hour
	if tod < windowStart || tod >= windowEnd {
		return s, fmt.Errorf("schedule time must be within IST 09:00–20:00")
	}
	if queryTimeoutSec < 1 {
		return s, fmt.Errorf("queryTimeoutSec must be positive")
	}
	if tod+time.Duration(queryTimeoutSec)*time.Second >= windowEnd {
		return s, fmt.Errorf("schedule time + queryTimeoutSec must finish before 20:00 IST")
	}
	switch strings.ToLower(strings.TrimSpace(s.Frequency)) {
	case "once":
		s.Frequency = "once"
		if strings.TrimSpace(s.OnceDate) == "" {
			return s, fmt.Errorf("onceDate is required for once frequency")
		}
		if _, err := time.ParseInLocation("2006-01-02", s.OnceDate, IST()); err != nil {
			return s, fmt.Errorf("onceDate must be YYYY-MM-DD")
		}
	case "daily":
		s.Frequency = "daily"
	case "weekly":
		s.Frequency = "weekly"
		if len(s.DaysOfWeek) == 0 {
			return s, fmt.Errorf("daysOfWeek is required for weekly frequency")
		}
		for _, d := range s.DaysOfWeek {
			if d < 1 || d > 7 {
				return s, fmt.Errorf("daysOfWeek values must be 1–7 (Mon–Sun)")
			}
		}
	case "monthly":
		s.Frequency = "monthly"
		if s.DayOfMonth < 1 || s.DayOfMonth > 31 {
			return s, fmt.Errorf("dayOfMonth must be 1–31")
		}
	default:
		return s, fmt.Errorf("frequency must be once|daily|weekly|monthly")
	}
	return s, nil
}

func parseHHMM(raw string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("schedule time must be HH:MM")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("schedule time hour is invalid")
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("schedule time minute is invalid")
	}
	return h, m, nil
}

// NextFireAt computes the next fire instant after `from` (exclusive of past).
func NextFireAt(s Schedule, from time.Time) (time.Time, error) {
	now := from.In(IST())
	hour, minute, err := parseHHMM(s.Time)
	if err != nil {
		return time.Time{}, err
	}
	switch s.Frequency {
	case "once":
		day, err := time.ParseInLocation("2006-01-02", s.OnceDate, IST())
		if err != nil {
			return time.Time{}, err
		}
		fire := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, IST())
		if !fire.After(now) {
			return time.Time{}, nil
		}
		return fire, nil
	case "daily":
		candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, IST())
		if !candidate.After(now) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate, nil
	case "weekly":
		allowed := map[int]bool{}
		for _, d := range s.DaysOfWeek {
			allowed[d] = true
		}
		for i := 0; i < 8; i++ {
			day := now.AddDate(0, 0, i)
			// Go Weekday: Sunday=0 … Saturday=6. Plan uses Mon=1 … Sun=7.
			wd := int(day.Weekday())
			if wd == 0 {
				wd = 7
			}
			if !allowed[wd] {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, IST())
			if candidate.After(now) {
				return candidate, nil
			}
		}
		return time.Time{}, fmt.Errorf("no weekly fire found")
	case "monthly":
		for i := 0; i < 14; i++ {
			base := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, IST()).AddDate(0, i, 0)
			lastDay := time.Date(base.Year(), base.Month()+1, 0, 0, 0, 0, 0, IST()).Day()
			dom := s.DayOfMonth
			if dom > lastDay {
				dom = lastDay
			}
			candidate := time.Date(base.Year(), base.Month(), dom, hour, minute, 0, 0, IST())
			if candidate.After(now) {
				return candidate, nil
			}
		}
		return time.Time{}, fmt.Errorf("no monthly fire found")
	default:
		return time.Time{}, fmt.Errorf("unknown frequency")
	}
}

// OnceDateBeforeToday reports whether a once campaign's onceDate is before today IST.
func OnceDateBeforeToday(s Schedule, now time.Time) bool {
	if s.Frequency != "once" {
		return false
	}
	day, err := time.ParseInLocation("2006-01-02", s.OnceDate, IST())
	if err != nil {
		return true
	}
	today := now.In(IST())
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, IST())
	return day.Before(today)
}

// ExpiresAtForScheduled returns 20:00 IST on the calendar date of scheduledAt.
func ExpiresAtForScheduled(scheduledAt time.Time) time.Time {
	ist := scheduledAt.In(IST())
	return time.Date(ist.Year(), ist.Month(), ist.Day(), 20, 0, 0, 0, IST())
}

// IsWithinSendWindow reports whether now is in [09:00, 20:00) IST.
func IsWithinSendWindow(now time.Time) bool {
	ist := now.In(IST())
	tod := time.Duration(ist.Hour())*time.Hour + time.Duration(ist.Minute())*time.Minute + time.Duration(ist.Second())*time.Second
	return tod >= 9*time.Hour && tod < 20*time.Hour
}

// FireWouldMissWindow is true when now + queryTimeout reaches or passes 20:00 IST.
func FireWouldMissWindow(now time.Time, queryTimeoutSec int) bool {
	ist := now.In(IST())
	end := time.Date(ist.Year(), ist.Month(), ist.Day(), 20, 0, 0, 0, IST())
	return !ist.Add(time.Duration(queryTimeoutSec) * time.Second).Before(end)
}
