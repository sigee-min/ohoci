package ocicredentials

import (
	"strings"

	"ohoci/internal/store"
)

func runtimeStatusFromConfig(runtime RuntimeConfig) (bool, []string) {
	missing := []string{}
	if strings.TrimSpace(runtime.CompartmentID) == "" {
		missing = append(missing, "OHOCI_OCI_COMPARTMENT_OCID")
	}
	if strings.TrimSpace(runtime.AvailabilityDomain) == "" {
		missing = append(missing, "OHOCI_OCI_AVAILABILITY_DOMAIN")
	}
	if strings.TrimSpace(runtime.SubnetID) == "" {
		missing = append(missing, "OHOCI_OCI_SUBNET_OCID")
	}
	if strings.TrimSpace(runtime.ImageID) == "" {
		missing = append(missing, "OHOCI_OCI_IMAGE_OCID")
	}
	return len(missing) == 0, missing
}

func sanitizeCredential(credential store.OCICredential) store.OCICredential {
	credential.PrivateKeyCiphertext = ""
	credential.PassphraseCiphertext = ""
	return credential
}

func normalizeDefaultMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "instance_principal":
		return "instance_principal"
	case "fake":
		return "fake"
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
