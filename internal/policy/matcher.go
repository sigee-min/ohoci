package policy

import (
	"slices"
	"strings"

	"ohoci/internal/store"
)

type MatchResult struct {
	Policy          *store.Policy
	RequestedLabels []string
}

type Explanation struct {
	Policy          *store.Policy `json:"policy,omitempty"`
	RequestedLabels []string      `json:"requestedLabels"`
	PolicyLabels    []string      `json:"policyLabels"`
	Matched         bool          `json:"matched"`
	Reasons         []string      `json:"reasons,omitempty"`
	MissingLabels   []string      `json:"missingLabels,omitempty"`
	ExtraLabels     []string      `json:"extraLabels,omitempty"`
}

func Match(policies []store.Policy, labels []string) MatchResult {
	requested := ManagedLabels(labels)
	for index := range policies {
		policy := policies[index]
		if !policy.Enabled {
			continue
		}
		if slices.Equal(Normalize(policy.Labels), requested) {
			return MatchResult{Policy: &policy, RequestedLabels: requested}
		}
	}
	return MatchResult{RequestedLabels: requested}
}

func Explain(policies []store.Policy, labels []string) []Explanation {
	requested := ManagedLabels(labels)
	items := make([]Explanation, 0, len(policies))
	for index := range policies {
		policy := policies[index]
		if !policy.Enabled {
			continue
		}
		policyLabels := Normalize(policy.Labels)
		explanation := Explanation{
			Policy:          &policy,
			RequestedLabels: requested,
			PolicyLabels:    policyLabels,
			Matched:         slices.Equal(policyLabels, requested),
		}
		if explanation.Matched {
			items = append(items, explanation)
			continue
		}
		explanation.MissingLabels = subtract(policyLabels, requested)
		explanation.ExtraLabels = subtract(requested, policyLabels)
		if len(explanation.MissingLabels) > 0 {
			explanation.Reasons = append(explanation.Reasons, "missing labels: "+strings.Join(explanation.MissingLabels, ", "))
		}
		if len(explanation.ExtraLabels) > 0 {
			explanation.Reasons = append(explanation.Reasons, "unexpected labels: "+strings.Join(explanation.ExtraLabels, ", "))
		}
		items = append(items, explanation)
	}
	return items
}

func ManagedLabels(labels []string) []string {
	out := Normalize(labels)
	filtered := out[:0]
	for _, label := range out {
		if label == "self-hosted" {
			continue
		}
		filtered = append(filtered, label)
	}
	return filtered
}

func Normalize(labels []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.ToLower(strings.TrimSpace(raw))
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	slices.Sort(out)
	return out
}

func subtract(left, right []string) []string {
	out := []string{}
	for _, item := range left {
		if slices.Contains(right, item) {
			continue
		}
		out = append(out, item)
	}
	return out
}
