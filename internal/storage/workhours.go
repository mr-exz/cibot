package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseWorkDays parses "1-5", "1,2,3,4,5", or "12345" into a map of weekday numbers
// 1=Monday, 7=Sunday (ISO 8601 convention)
func ParseWorkDays(s string) (map[int]bool, error) {
	result := make(map[int]bool)

	if s == "" {
		return result, nil
	}

	// Check if it's a range like "1-5"
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format: %s", s)
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid range numbers: %s", s)
		}
		for i := start; i <= end; i++ {
			if i < 1 || i > 7 {
				return nil, fmt.Errorf("day number out of range: %d", i)
			}
			result[i] = true
		}
		return result, nil
	}

	// Check if it's comma-separated like "1,2,3,4,5"
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		for _, part := range parts {
			day, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return nil, fmt.Errorf("invalid day number: %s", part)
			}
			if day < 1 || day > 7 {
				return nil, fmt.Errorf("day number out of range: %d", day)
			}
			result[day] = true
		}
		return result, nil
	}

	// Otherwise, parse as individual digits like "12345"
	for _, ch := range s {
		day, err := strconv.Atoi(string(ch))
		if err != nil {
			return nil, fmt.Errorf("invalid day digit: %c", ch)
		}
		if day < 1 || day > 7 {
			return nil, fmt.Errorf("day number out of range: %d", day)
		}
		result[day] = true
	}

	return result, nil
}

// ParseWorkHours parses "08:30-18:30" into (startMinutes, endMinutes, error)
// Returns minutes since midnight
func ParseWorkHours(s string) (int, int, error) {
	if s == "" {
		return 0, 0, nil
	}

	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid work hours format: %s", s)
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	startMin, err1 := parseTimeToMinutes(startStr)
	endMin, err2 := parseTimeToMinutes(endStr)
	if err1 != nil {
		return 0, 0, fmt.Errorf("invalid start time: %s", startStr)
	}
	if err2 != nil {
		return 0, 0, fmt.Errorf("invalid end time: %s", endStr)
	}

	return startMin, endMin, nil
}

// parseTimeToMinutes converts "HH:MM" to minutes since midnight
func parseTimeToMinutes(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	hours, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	minutes, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))

	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("invalid time numbers in: %s", timeStr)
	}

	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return 0, fmt.Errorf("time out of range: %02d:%02d", hours, minutes)
	}

	return hours*60 + minutes, nil
}

// ParseTimezone parses "+02:00" or "-05:30" into a *time.Location
func ParseTimezone(s string) (*time.Location, error) {
	if s == "" {
		return time.UTC, nil
	}

	// Parse offset string like "+02:00" or "-05:30"
	s = strings.TrimSpace(s)

	// Get sign
	if len(s) < 1 {
		return nil, fmt.Errorf("empty timezone string")
	}

	var sign int
	var offsetStr string
	if s[0] == '+' {
		sign = 1
		offsetStr = s[1:]
	} else if s[0] == '-' {
		sign = -1
		offsetStr = s[1:]
	} else {
		return nil, fmt.Errorf("invalid timezone format: must start with + or -")
	}

	// Parse HH:MM or HHMM
	var hours, minutes int
	var err error

	if strings.Contains(offsetStr, ":") {
		parts := strings.Split(offsetStr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid timezone format: %s", s)
		}
		hours, err = strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid timezone hours: %s", parts[0])
		}
		minutes, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid timezone minutes: %s", parts[1])
		}
	} else {
		// Try parsing as HHMM
		if len(offsetStr) < 2 {
			return nil, fmt.Errorf("invalid timezone format: %s", s)
		}
		hours, err = strconv.Atoi(offsetStr[:2])
		if err != nil {
			return nil, fmt.Errorf("invalid timezone hours: %s", offsetStr[:2])
		}
		if len(offsetStr) > 2 {
			minutes, err = strconv.Atoi(offsetStr[2:])
			if err != nil {
				return nil, fmt.Errorf("invalid timezone minutes: %s", offsetStr[2:])
			}
		}
	}

	offsetSeconds := sign * (hours*3600 + minutes*60)
	return time.FixedZone("", offsetSeconds), nil
}

// IsPersonOnline returns true if the person is currently within their working hours
// Returns true (always available) if any of the three fields is empty
func IsPersonOnline(p SupportPerson, now time.Time) bool {
	// If any working field is empty, person is always available
	if p.Timezone == "" || p.WorkHours == "" || p.WorkDays == "" {
		return true
	}

	// Parse timezone
	loc, err := ParseTimezone(p.Timezone)
	if err != nil {
		// If timezone parsing fails, assume available
		return true
	}

	// Convert to person's local time
	localNow := now.In(loc)

	// Check work days
	days, err := ParseWorkDays(p.WorkDays)
	if err != nil {
		// If parsing fails, assume available
		return true
	}

	// Go's Weekday: Sunday=0, Monday=1, ..., Saturday=6
	// ISO 8601: Monday=1, ..., Sunday=7
	isoDay := int(localNow.Weekday())
	if isoDay == 0 {
		isoDay = 7 // Convert Sunday from 0 to 7
	}

	if !days[isoDay] {
		// Not a working day
		return false
	}

	// Check work hours
	startMin, endMin, err := ParseWorkHours(p.WorkHours)
	if err != nil {
		// If parsing fails, assume available
		return true
	}

	currentMin := localNow.Hour()*60 + localNow.Minute()
	return currentMin >= startMin && currentMin < endMin
}
