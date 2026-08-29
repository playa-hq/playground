package main

import (
	"fmt"
	"strings"
)

// Numeric axes are phrased from the axis itself rather than through a generic
// "higher X" template. "Which has the higher founded year?" is technically
// correct and reads like a database; "Which was founded first?" reads like a
// quiz. Years also need era handling — a founding date of -218 is 218 BC, not
// minus two hundred and eighteen.

// isYearAxis reports whether an axis holds a calendar year.
func isYearAxis(key string) bool {
	k := strings.ToLower(key)
	return strings.HasSuffix(k, "_year") || k == "year" || strings.HasSuffix(k, "_date") ||
		strings.Contains(k, "found") || strings.Contains(k, "launch") ||
		strings.Contains(k, "incorporat") || strings.Contains(k, "birth")
}

// numericPrompt returns the question text and whether the answer is the
// *lower* value. For years the natural question is "which came first", which
// inverts what counts as the winning side.
func numericPrompt(label, key string) (prompt string, lowerWins bool) {
	if isYearAxis(key) {
		switch {
		case strings.Contains(strings.ToLower(key), "launch"):
			return "Which launched first?", true
		case strings.Contains(strings.ToLower(key), "found"), strings.Contains(strings.ToLower(key), "incorporat"):
			return "Which was founded first?", true
		case strings.Contains(strings.ToLower(key), "birth"):
			return "Who was born first?", true
		}
		return fmt.Sprintf("Which has the earlier %s?", strings.ToLower(label)), true
	}
	return fmt.Sprintf("Which has the higher %s?", strings.ToLower(label)), false
}

// formatValue renders a value for the reveal line, with eras on years.
func formatValue(key string, v float64) string {
	if isYearAxis(key) {
		year := int64(v)
		if year < 0 {
			return fmt.Sprintf("%d BC", -year)
		}
		return fmt.Sprintf("%d", year)
	}
	return formatNum(v)
}

// formatUnit renders a grounded number the way a person would say it:
// "$45.3B" for a USD metric, "166,000" for a headcount, an era for a year.
func formatUnit(key string, v float64, unit string) string {
	if isYearAxis(key) {
		return formatValue(key, v)
	}
	prefix := ""
	switch strings.ToUpper(unit) {
	case "USD":
		prefix = "$"
	case "EUR":
		prefix = "€"
	case "GBP":
		prefix = "£"
	}
	abs := v
	if abs < 0 {
		abs = -abs
	}
	sign := ""
	if v < 0 {
		sign = "-"
	}
	switch {
	case abs >= 1e12:
		return fmt.Sprintf("%s%s%.1fT", sign, prefix, abs/1e12)
	case abs >= 1e9:
		return fmt.Sprintf("%s%s%.1fB", sign, prefix, abs/1e9)
	case abs >= 1e6:
		return fmt.Sprintf("%s%s%.1fM", sign, prefix, abs/1e6)
	}
	return sign + prefix + groupThousands(formatNum(abs))
}

func groupThousands(s string) string {
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + frac
}
