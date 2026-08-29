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
	return strings.HasSuffix(k, "_year") || k == "year" ||
		strings.Contains(k, "founded") || strings.Contains(k, "launch")
}

// numericPrompt returns the question text and whether the answer is the
// *lower* value. For years the natural question is "which came first", which
// inverts what counts as the winning side.
func numericPrompt(label, key string) (prompt string, lowerWins bool) {
	if isYearAxis(key) {
		switch {
		case strings.Contains(strings.ToLower(key), "launch"):
			return "Which launched first?", true
		case strings.Contains(strings.ToLower(key), "founded"):
			return "Which was founded first?", true
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
