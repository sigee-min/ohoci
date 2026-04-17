package ocicredentials

import (
	"context"
	"testing"

	"ohoci/internal/store"

	"github.com/oracle/oci-go-sdk/v65/common"
)

type staticTester struct {
	regions []string
	ads     []string
	err     error
}

func (s staticTester) Test(_ context.Context, _ common.ConfigurationProvider, _ string) ([]string, []string, error) {
	return s.regions, s.ads, s.err
}

func TestServiceSaveAndResolveProvider(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := New(db, Config{
		DefaultMode:   "fake",
		EncryptionKey: "top-secret",
		Runtime: RuntimeConfig{
			CompartmentID:      "ocid1.compartment.oc1..example",
			AvailabilityDomain: "kIdk:AP-SEOUL-1-AD-1",
			SubnetID:           "ocid1.subnet.oc1..example",
			ImageID:            "ocid1.image.oc1..example",
		},
		Tester: staticTester{
			regions: []string{"ap-seoul-1"},
			ads:     []string{"kIdk:AP-SEOUL-1-AD-1"},
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	input := Input{
		Name:        "Tenant A",
		ProfileName: "DEFAULT",
		ConfigText: `
[DEFAULT]
user=ocid1.user.oc1..user
fingerprint=11:22:33:44
tenancy=ocid1.tenancy.oc1..tenancy
region=ap-seoul-1
`,
		PrivateKeyPEM: testPrivateKey,
	}

	result, err := service.Save(ctx, input)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !result.Credential.IsActive {
		t.Fatalf("expected stored credential to be active")
	}
	if result.Credential.TenancyOCID != "ocid1.tenancy.oc1..tenancy" {
		t.Fatalf("unexpected tenancy OCID: %q", result.Credential.TenancyOCID)
	}

	record, err := db.FindActiveOCICredential(ctx)
	if err != nil {
		t.Fatalf("find active credential: %v", err)
	}
	if record.PrivateKeyCiphertext == "" {
		t.Fatalf("expected encrypted private key")
	}
	if record.PrivateKeyCiphertext == testPrivateKey {
		t.Fatalf("expected encrypted private key, got plaintext")
	}

	provider, ok, err := service.ResolveProvider(ctx)
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	if !ok {
		t.Fatalf("expected active provider")
	}
	tenancyOCID, err := provider.TenancyOCID()
	if err != nil {
		t.Fatalf("provider tenancy: %v", err)
	}
	if tenancyOCID != "ocid1.tenancy.oc1..tenancy" {
		t.Fatalf("unexpected provider tenancy: %q", tenancyOCID)
	}
}

func TestServiceCurrentStatusUsesDefaultModeWithoutCredential(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := New(db, Config{
		DefaultMode:   "instance_principal",
		EncryptionKey: "top-secret",
		Tester:        staticTester{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if status.EffectiveMode != "instance_principal" {
		t.Fatalf("unexpected effective mode: %q", status.EffectiveMode)
	}
	if status.ActiveCredential != nil {
		t.Fatalf("expected no active credential")
	}
}

func TestParseInputFallsBackToFirstProfile(t *testing.T) {
	input := Input{
		ConfigText: `
[TEAM_A]
user=ocid1.user.oc1..user
fingerprint=11:22:33:44
tenancy=ocid1.tenancy.oc1..tenancy
region=ap-seoul-1
`,
		PrivateKeyPEM: testPrivateKey,
	}

	parsed, err := parseInput(input)
	if err != nil {
		t.Fatalf("parse input: %v", err)
	}
	if parsed.ProfileName != "TEAM_A" {
		t.Fatalf("unexpected profile name: %q", parsed.ProfileName)
	}
	if parsed.Name != "OCI TEAM_A" {
		t.Fatalf("unexpected generated name: %q", parsed.Name)
	}
}

const testPrivateKey = "-----BEGIN PRIVATE KEY-----\nMIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAOW0ED5HhOi+am89\n+A8Gs84lcTxj95fyY/m4El01AaOMwB6Ufnx8lIIY7abn71exSaKDzsFNEM+uBkdH\nW8mG+Lna3TGmRS52G46DnulBiREnpRV+NIQwMjZHpQ5WvW9nzePZ4navmdnhyrcE\npYA3vKJKND/p8+8mlD0G8CfD0Ko3AgMBAAECgYA1HvMys+90s7SBjV80emRSpC4P\nvT6hERk1wu/cRknevMohSE4IE/d0LrenBbRAH2vb/YdvBJeCr8gb69C6RlB2mo25\ngMv8A+zggDGyIJEq5JCIGsFWa463bd8P/Y+tZ6ZsCULVuksWl+suvhoJvr3zBeeM\neQMF3rd8hzhYa5iqYQJBAPakEcZAcMAQWcjzBQKmdZoP+zXvExMOrDlFKeqsbeWP\nVHFrpcZ+t/A3SwKKOmX5Ie50rPtCBi+2NfLYYebGnv0CQQDua3Vvomv1zyJmuEi+\nHr+rqHtzjjA8vVUCK8Tb9UEqWLZ3JQNcoGvgHUZrw3Euq1nqvOYYHsZGTLXSIrlu\nwaZDAkBa+tSvq++reZyVGsgbXSn+ZazGDWWc3wm6qn+22FpFluSQXiQtn2rcipj5\n2+GE4iyZGKMCoC1GBlHKPfWHOndFAkAwso44EQrQGFDEfluNSaaIn08n2SENJvbY\nDKyW6M84oQoT5+F55+Jg0lnx5OeXSrSA97hfsNl6vmxc0W7iqncVAkEArERxQtrn\nd/fYemHb9Wv5ibLOZWoPCNy2WACMGyHQ7+3+pB/IxI9ueUrnrRaCAQLkDuhF82sW\nnApG0TpVWHyZUQ==\n-----END PRIVATE KEY-----"
