package policy

import (
	"testing"

	"ohoci/internal/store"
)

func TestMatchIgnoresSelfHostedAndNormalizes(t *testing.T) {
	policies := []store.Policy{
		{ID: 1, Labels: []string{"oci", "cpu"}, Enabled: true},
	}
	result := Match(policies, []string{"self-hosted", "CPU", "oci", "oci"})
	if result.Policy == nil || result.Policy.ID != 1 {
		t.Fatalf("expected policy 1 match, got %#v", result.Policy)
	}
}
