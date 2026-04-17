package ocicredentials

import (
	"context"
	"crypto/sha256"

	"ohoci/internal/store"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type RuntimeConfig struct {
	CompartmentID      string
	AvailabilityDomain string
	SubnetID           string
	ImageID            string
}

type Config struct {
	DefaultMode           string
	EncryptionKey         string
	Runtime               RuntimeConfig
	RuntimeStatusProvider RuntimeStatusProvider
	Tester                ConnectionTester
}

type Input struct {
	Name          string `json:"name"`
	ProfileName   string `json:"profileName"`
	ConfigText    string `json:"configText"`
	PrivateKeyPEM string `json:"privateKeyPem"`
	Passphrase    string `json:"passphrase"`
}

type InspectInput struct {
	Name        string `json:"name"`
	ProfileName string `json:"profileName"`
	ConfigText  string `json:"configText"`
}

type InspectResult struct {
	Profiles              []string `json:"profiles"`
	SelectedProfile       string   `json:"selectedProfile"`
	SuggestedName         string   `json:"suggestedName"`
	TenancyOCID           string   `json:"tenancyOcid"`
	UserOCID              string   `json:"userOcid"`
	Fingerprint           string   `json:"fingerprint"`
	Region                string   `json:"region"`
	HasEmbeddedPrivateKey bool     `json:"hasEmbeddedPrivateKey"`
	HasPassphrase         bool     `json:"hasPassphrase"`
}

type Status struct {
	EffectiveMode        string               `json:"effectiveMode"`
	DefaultMode          string               `json:"defaultMode"`
	ActiveCredential     *store.OCICredential `json:"activeCredential,omitempty"`
	RuntimeConfigReady   bool                 `json:"runtimeConfigReady"`
	RuntimeConfigMissing []string             `json:"runtimeConfigMissing,omitempty"`
}

type TestResult struct {
	EffectiveMode        string              `json:"effectiveMode"`
	Credential           store.OCICredential `json:"credential"`
	RegionSubscriptions  []string            `json:"regionSubscriptions,omitempty"`
	AvailabilityDomains  []string            `json:"availabilityDomains,omitempty"`
	Message              string              `json:"message"`
	RuntimeConfigReady   bool                `json:"runtimeConfigReady"`
	RuntimeConfigMissing []string            `json:"runtimeConfigMissing,omitempty"`
}

type ConnectionTester interface {
	Test(ctx context.Context, provider common.ConfigurationProvider, tenancyOCID string) ([]string, []string, error)
}

type RuntimeStatusProvider interface {
	RuntimeStatus(ctx context.Context) (bool, []string, error)
}

type Service struct {
	store                 *store.Store
	key                   [32]byte
	defaultMode           string
	runtime               RuntimeConfig
	runtimeStatusProvider RuntimeStatusProvider
	tester                ConnectionTester
}

type identityConnectionTester struct{}

type parsedCredential struct {
	Name          string
	Profiles      []string
	ProfileName   string
	TenancyOCID   string
	UserOCID      string
	Fingerprint   string
	Region        string
	PrivateKeyPEM string
	Passphrase    string
}

var sha256Sum = sha256.Sum256
