package config

import (
	"sort"
	"strconv"
	"strings"
)

// ParseRangeList reads a comma/space-separated list of 1-based range numbers.
func ParseRangeList(s string) []int {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	seen := make(map[int]bool, len(parts))
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// FormatRangeList writes range numbers as a compact comma-separated list.
func FormatRangeList(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	seen := make(map[int]bool, len(nums))
	clean := make([]int, 0, len(nums))
	for _, n := range nums {
		if n < 1 || seen[n] {
			continue
		}
		seen[n] = true
		clean = append(clean, n)
	}
	sort.Ints(clean)
	parts := make([]string, len(clean))
	for i, n := range clean {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}
