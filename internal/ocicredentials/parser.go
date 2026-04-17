package ocicredentials

import (
	"bufio"
	"fmt"
	"strings"
)

func parseInput(input Input) (parsedCredential, error) {
	credential, err := inspectCredential(InspectInput{
		Name:        input.Name,
		ProfileName: input.ProfileName,
		ConfigText:  input.ConfigText,
	})
	if err != nil {
		return parsedCredential{}, err
	}
	profiles, _, err := parseOCIProfiles(input.ConfigText)
	if err != nil {
		return parsedCredential{}, err
	}
	selectedProfile := profiles[credential.ProfileName]
	privateKeyPEM := normalizePEM(firstNonEmpty(strings.TrimSpace(input.PrivateKeyPEM), selectedProfile["key_content"]))
	if privateKeyPEM == "" {
		return parsedCredential{}, fmt.Errorf("private key file is required")
	}
	credential.PrivateKeyPEM = privateKeyPEM
	credential.Passphrase = strings.TrimSpace(firstNonEmpty(input.Passphrase, selectedProfile["pass_phrase"], selectedProfile["passphrase"]))
	return credential, nil
}

func inspectCredential(input InspectInput) (parsedCredential, error) {
	profiles, order, err := parseOCIProfiles(input.ConfigText)
	if err != nil {
		return parsedCredential{}, err
	}
	profileName, profile, err := selectProfile(profiles, order, input.ProfileName)
	if err != nil {
		return parsedCredential{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = fmt.Sprintf("OCI %s", profileName)
	}
	credential := parsedCredential{
		Name:        name,
		Profiles:    append([]string(nil), order...),
		ProfileName: profileName,
		TenancyOCID: strings.TrimSpace(profile["tenancy"]),
		UserOCID:    strings.TrimSpace(profile["user"]),
		Fingerprint: strings.TrimSpace(profile["fingerprint"]),
		Region:      strings.TrimSpace(profile["region"]),
	}
	switch {
	case credential.TenancyOCID == "":
		return parsedCredential{}, fmt.Errorf("config is missing tenancy")
	case credential.UserOCID == "":
		return parsedCredential{}, fmt.Errorf("config is missing user")
	case credential.Fingerprint == "":
		return parsedCredential{}, fmt.Errorf("config is missing fingerprint")
	case credential.Region == "":
		return parsedCredential{}, fmt.Errorf("config is missing region")
	}
	return credential, nil
}

func parseOCIProfiles(configText string) (map[string]map[string]string, []string, error) {
	scanner := bufio.NewScanner(strings.NewReader(configText))
	profiles := map[string]map[string]string{}
	order := []string{}
	currentProfile := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentProfile = strings.TrimSpace(line[1 : len(line)-1])
			if currentProfile == "" {
				return nil, nil, fmt.Errorf("config contains an empty profile name")
			}
			if _, exists := profiles[currentProfile]; !exists {
				profiles[currentProfile] = map[string]string{}
				order = append(order, currentProfile)
			}
			continue
		}
		if currentProfile == "" {
			return nil, nil, fmt.Errorf("config must start with a profile section")
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, nil, fmt.Errorf("invalid config line %q", line)
		}
		profiles[currentProfile][strings.ToLower(strings.TrimSpace(key))] = normalizeConfigValue(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if len(order) == 0 {
		return nil, nil, fmt.Errorf("config does not contain any profiles")
	}
	return profiles, order, nil
}

func selectProfile(profiles map[string]map[string]string, order []string, requested string) (string, map[string]string, error) {
	name := strings.TrimSpace(requested)
	if name != "" {
		for profileName, profile := range profiles {
			if strings.EqualFold(profileName, name) {
				return profileName, profile, nil
			}
		}
		return "", nil, fmt.Errorf("profile %s not found in config", name)
	}
	for profileName, profile := range profiles {
		if strings.EqualFold(profileName, "DEFAULT") {
			return profileName, profile, nil
		}
	}
	first := order[0]
	return first, profiles[first], nil
}

func normalizeConfigValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if (strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"")) || (strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'")) {
			trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		}
	}
	return strings.TrimSpace(trimmed)
}

func normalizePEM(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	if strings.Contains(normalized, `\n`) && !strings.Contains(normalized, "\n") {
		normalized = strings.ReplaceAll(normalized, `\n`, "\n")
	}
	return normalized
}
