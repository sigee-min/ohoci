package store

import (
	"encoding/json"
	"slices"
	"strings"
)

func normalizeLabels(labels []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.ToLower(strings.TrimSpace(raw))
		if label == "" {
			continue
		}
		if _, exists := set[label]; exists {
			continue
		}
		set[label] = struct{}{}
		out = append(out, label)
	}
	slices.Sort(out)
	return out
}

func normalizeStrings(values []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, exists := set[value]; exists {
			continue
		}
		set[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func normalizeCommands(values []string) []string {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func boolAsInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func marshalJSONArray(values []string) (string, error) {
	out, err := json.Marshal(normalizeStrings(values))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func unmarshalJSONArray(raw string, target *[]string) error {
	if target == nil {
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		*target = nil
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}

func isIgnorableMigrationError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "duplicate column") || strings.Contains(message, "1060")
}
