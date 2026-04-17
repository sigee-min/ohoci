package ocibilling

import (
	"context"
	"testing"
	"time"

	"ohoci/internal/oci"
	"ohoci/internal/store"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/usageapi"
)

type providerResolverFunc func(ctx context.Context) (common.ConfigurationProvider, bool, error)

func (fn providerResolverFunc) ResolveProvider(ctx context.Context) (common.ConfigurationProvider, bool, error) {
	return fn(ctx)
}

type usageClientStub struct {
	trackedRows []usageapi.UsageSummary
	overallRows []usageapi.UsageSummary
	requests    []usageapi.RequestSummarizedUsagesRequest
}

func (stub *usageClientStub) RequestSummarizedUsages(_ context.Context, request usageapi.RequestSummarizedUsagesRequest) (usageapi.RequestSummarizedUsagesResponse, error) {
	stub.requests = append(stub.requests, request)
	rows := stub.trackedRows
	if request.RequestSummarizedUsagesDetails.Filter == nil {
		rows = stub.overallRows
	}
	return usageapi.RequestSummarizedUsagesResponse{
		UsageAggregation: usageapi.UsageAggregation{
			Items: rows,
		},
	}, nil
}

func TestServicePolicyBreakdownVerifiesTagsAndFallsBackSafely(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/billing.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policy, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       1,
		MemoryGB:   16,
		MaxRunners: 2,
		TTLMinutes: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	windowEnd := time.Now().UTC()
	windowStart := windowEnd.AddDate(0, 0, -7)
	launchedAt := windowStart.Add(2 * time.Hour)
	expiresAt := windowEnd.Add(2 * time.Hour)
	if _, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:       policy.ID,
		JobID:          1,
		InstallationID: 1,
		InstanceOCID:   "ocid1.instance.oc1..verified",
		RepoOwner:      "example",
		RepoName:       "repo-verified",
		RunnerName:     "runner-verified",
		Status:         "completed",
		Labels:         []string{"self-hosted", "oci", "cpu"},
		LaunchedAt:     &launchedAt,
		ExpiresAt:      &expiresAt,
	}); err != nil {
		t.Fatalf("create verified runner: %v", err)
	}
	if _, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:       policy.ID,
		JobID:          2,
		InstallationID: 1,
		InstanceOCID:   "ocid1.instance.oc1..fallback",
		RepoOwner:      "example",
		RepoName:       "repo-fallback",
		RunnerName:     "runner-fallback",
		Status:         "completed",
		Labels:         []string{"self-hosted", "oci", "cpu"},
		LaunchedAt:     &launchedAt,
		ExpiresAt:      &expiresAt,
	}); err != nil {
		t.Fatalf("create fallback runner: %v", err)
	}

	provider := common.NewRawConfigurationProvider(
		"ocid1.tenancy.oc1..example",
		"ocid1.user.oc1..example",
		"us-phoenix-1",
		"fingerprint",
		testPrivateKeyPEM,
		nil,
	)
	usageClient := &usageClientStub{
		trackedRows: []usageapi.UsageSummary{
			{
				ResourceId:       common.String("ocid1.instance.oc1..verified"),
				ComputedAmount:   common.Float32(1.25),
				ComputedQuantity: common.Float32(4),
				Unit:             common.String("OCPU Hours"),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
				Tags: []usageapi.Tag{{
					Namespace: common.String("ohoci"),
					Key:       common.String(oci.BillingDefinedTagKeyPolicyID),
					Value:     common.String("1"),
				}},
			},
			{
				ResourceId:       common.String("ocid1.instance.oc1..fallback"),
				ComputedAmount:   common.Float32(0.5),
				ComputedQuantity: common.Float32(2),
				Unit:             common.String("OCPU Hours"),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
			},
			{
				ResourceId:       common.String("ocid1.instance.oc1..tagonly"),
				ComputedAmount:   common.Float32(0.25),
				ComputedQuantity: common.Float32(1),
				Unit:             common.String("OCPU Hours"),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
				Tags: []usageapi.Tag{{
					Namespace: common.String("ohoci"),
					Key:       common.String(oci.BillingDefinedTagKeyPolicyID),
					Value:     common.String("1"),
				}},
			},
			{
				ResourceId:       common.String("ocid1.instance.oc1..stale"),
				ComputedAmount:   common.Float32(0.75),
				ComputedQuantity: common.Float32(2),
				Unit:             common.String("OCPU Hours"),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
				Tags: []usageapi.Tag{{
					Namespace: common.String("ohoci"),
					Key:       common.String(oci.BillingDefinedTagKeyPolicyID),
					Value:     common.String("999"),
				}},
			},
		},
		overallRows: []usageapi.UsageSummary{
			{
				ComputedAmount:   common.Float32(1.25),
				ComputedQuantity: common.Float32(4),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
			},
			{
				ComputedAmount:   common.Float32(0.5),
				ComputedQuantity: common.Float32(2),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
			},
			{
				ComputedAmount:   common.Float32(0.25),
				ComputedQuantity: common.Float32(1),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
			},
			{
				ComputedAmount:   common.Float32(0.75),
				ComputedQuantity: common.Float32(2),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
			},
			{
				ComputedAmount:   common.Float32(1.75),
				ComputedQuantity: common.Float32(3),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: launchedAt},
				TimeUsageEnded:   &common.SDKTime{Time: launchedAt.Add(24 * time.Hour)},
			},
		},
	}
	service, err := New(db, Config{
		DefaultMode:         "instance_principal",
		BillingTagNamespace: "ohoci",
		ProviderResolver: providerResolverFunc(func(context.Context) (common.ConfigurationProvider, bool, error) {
			return provider, true, nil
		}),
		UsageClientFactory: func(common.ConfigurationProvider) (UsageClient, error) {
			return usageClient, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	report, err := service.PolicyBreakdown(ctx, PolicyBreakdownRequest{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	if err != nil {
		t.Fatalf("policy breakdown: %v", err)
	}

	if report.TotalCost != 2.75 {
		t.Fatalf("expected total cost 2.75, got %g", report.TotalCost)
	}
	if report.OCIBilledCost != 4.5 {
		t.Fatalf("expected OCI billed cost 4.5, got %g", report.OCIBilledCost)
	}
	if report.TagVerifiedCost != 1.25 {
		t.Fatalf("expected tag verified cost 1.25, got %g", report.TagVerifiedCost)
	}
	if report.ResourceFallbackCost != 0.5 {
		t.Fatalf("expected resource fallback cost 0.5, got %g", report.ResourceFallbackCost)
	}
	if report.TagOnlyCost != 0.25 {
		t.Fatalf("expected tag-only cost 0.25, got %g", report.TagOnlyCost)
	}
	if report.UnmappedCost != 0.75 {
		t.Fatalf("expected unmapped cost 0.75, got %g", report.UnmappedCost)
	}
	if len(report.Items) != 3 {
		t.Fatalf("expected 3 mapped policy items, got %d", len(report.Items))
	}
	if report.Items[0].PolicyID != policy.ID {
		t.Fatalf("expected mapped policy ID %d, got %d", policy.ID, report.Items[0].PolicyID)
	}
	if report.Items[0].AttributionStatus != "tag_verified" {
		t.Fatalf("expected tag_verified status for verified repo, got %q", report.Items[0].AttributionStatus)
	}
	if report.Items[1].AttributionStatus != "resource_fallback" {
		t.Fatalf("expected resource_fallback status for fallback repo, got %q", report.Items[1].AttributionStatus)
	}
	if report.Items[2].AttributionStatus != "tag_only" {
		t.Fatalf("expected tag_only status for tag-only repo, got %q", report.Items[2].AttributionStatus)
	}
	if len(report.Issues) != 3 {
		t.Fatalf("expected 3 billing issues, got %d", len(report.Issues))
	}
	reasons := map[string]bool{}
	for _, issue := range report.Issues {
		reasons[issue.Reason] = true
	}
	if !reasons["stale_policy_tag_without_tracked_runner"] {
		t.Fatalf("expected stale policy tag issue, got %#v", report.Issues)
	}
	if !reasons["tag_only_without_tracked_runner"] {
		t.Fatalf("expected tag-only issue, got %#v", report.Issues)
	}
	if !reasons["missing_policy_tag_resource_fallback"] {
		t.Fatalf("expected fallback issue for missing tag, got %#v", report.Issues)
	}
	if len(usageClient.requests) != 2 {
		t.Fatalf("expected one OCI total request and one tracked request, got %d", len(usageClient.requests))
	}
	if usageClient.requests[0].RequestSummarizedUsagesDetails.Filter != nil {
		t.Fatalf("expected OCI total request without resource filter")
	}
	if len(usageClient.requests[0].RequestSummarizedUsagesDetails.GroupBy) != 0 {
		t.Fatalf("expected OCI total request without groupBy, got %#v", usageClient.requests[0].RequestSummarizedUsagesDetails.GroupBy)
	}
	if usageClient.requests[1].RequestSummarizedUsagesDetails.Filter == nil {
		t.Fatalf("expected tracked request to include resource filter")
	}
	if len(usageClient.requests[1].RequestSummarizedUsagesDetails.GroupBy) != 1 ||
		usageClient.requests[1].RequestSummarizedUsagesDetails.GroupBy[0] != "resourceId" {
		t.Fatalf("expected tracked request grouped by resourceId, got %#v", usageClient.requests[1].RequestSummarizedUsagesDetails.GroupBy)
	}
}

func TestServicePolicyBreakdownIncludesOCIBilledCostWithoutTrackedRunners(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/billing.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	provider := common.NewRawConfigurationProvider(
		"ocid1.tenancy.oc1..example",
		"ocid1.user.oc1..example",
		"us-phoenix-1",
		"fingerprint",
		testPrivateKeyPEM,
		nil,
	)
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.AddDate(0, 0, -7)
	usageClient := &usageClientStub{
		overallRows: []usageapi.UsageSummary{
			{
				ComputedAmount:   common.Float32(3.25),
				ComputedQuantity: common.Float32(4),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: windowStart},
				TimeUsageEnded:   &common.SDKTime{Time: windowStart.Add(24 * time.Hour)},
			},
			{
				ComputedAmount:   common.Float32(1.75),
				ComputedQuantity: common.Float32(2),
				Currency:         common.String("USD"),
				TimeUsageStarted: &common.SDKTime{Time: windowStart.Add(24 * time.Hour)},
				TimeUsageEnded:   &common.SDKTime{Time: windowStart.Add(48 * time.Hour)},
			},
		},
	}
	service, err := New(db, Config{
		DefaultMode:         "instance_principal",
		BillingTagNamespace: "ohoci",
		ProviderResolver: providerResolverFunc(func(context.Context) (common.ConfigurationProvider, bool, error) {
			return provider, true, nil
		}),
		UsageClientFactory: func(common.ConfigurationProvider) (UsageClient, error) {
			return usageClient, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	report, err := service.PolicyBreakdown(ctx, PolicyBreakdownRequest{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	if err != nil {
		t.Fatalf("policy breakdown: %v", err)
	}

	if report.OCIBilledCost != 5 {
		t.Fatalf("expected OCI billed cost 5, got %g", report.OCIBilledCost)
	}
	if report.TotalCost != 0 {
		t.Fatalf("expected tracked cost 0 without runners, got %g", report.TotalCost)
	}
	if report.Currency != "USD" {
		t.Fatalf("expected currency USD from OCI total query, got %q", report.Currency)
	}
	if len(report.Items) != 0 {
		t.Fatalf("expected no tracked items, got %d", len(report.Items))
	}
	if len(usageClient.requests) != 1 {
		t.Fatalf("expected only OCI total request without tracked runners, got %d", len(usageClient.requests))
	}
	if usageClient.requests[0].RequestSummarizedUsagesDetails.Filter != nil {
		t.Fatalf("expected OCI total request without resource filter")
	}
}

const testPrivateKeyPEM = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEAzzq4VhVg3yX6iMUiP+6JGZveHFkNjEcNWef39/C4R2tQeM+c\nN6i/K3PaY5E9O+V1YxDCEV4VpWw2X2gYdEx+kt1/3uzMdGII4XESyqSeX5TR1+t0\nmno0RvrRZtGJPD7W82dManIeZDV4SSQdlqzTeWY5Avzkdxl3pNGdisz8Iky3UczZ\n7YT1Do1B4ezgmO6ijLJrVN6a8GN28AL5OHnqd7qV3CyMfCVxYvBy06SnVAk0nnBY\nuKxR271GGBqBPdZiZsaAJ+lZeI7IuAv89xDSHLVQQDBlnJrped1IovnEwlHGawEq\n3OC/YLXTr4Wr9PXgCulvRlQCxwBEm90LkAMPxQIDAQABAoIBAHxMvmXrKE+g6u40\nCMgfHdCqkfNNpJBWlAbIYW/W2PASi6DPd7OJbRRqtD9h5pz2jdK5Zk90un0nLBK/\nXn1HULICwhf66A1VpzwuNFuIBqmoeZaZX6mE6xPD58Ll35H5TADaBrZEcD3xKhsR\nHIX66vepQP9en5ZaY1f3T5iAG2wE8xmPKzW0fvkYlEYwH14r2raXIiQunlslqY5T\nr8j04YfGLwRoTSesFiNUFDXL9uBeb5GsyhQOdE31x4n4t4DccnE9vrFica8FWHQr\nizxgxYkWwaP42gnikIze8ih/7gToYtL6vhfVqlhK/SXxPxq8np5xpoE2mR7Bfrps\nR9f7D78CgYEA/0UUfZo0fo0RKZ77N8BINU3CFAaj6MqFmoVoeAQMtk0iQF5EYici\nYOEoTP0TO+cdMKx+3CX6X1cxZV8oPZxI25f6/sOlne342XQI3/OQvPsvBLmvBxxt\n+rsoAhH2m6q+UpBAkLTlaX2UjsFvdcGaVbL50DJBSSTXobHxPPH/Fk8CgYEA0t2b\nM+c/2ymlk/wEYBLETymFcpnSUsctNk6heAQ7EzxKQEiC5EvhczHxVn9Yx8RJWb1x\n1o1t4bm/FvGV8eK3opgDdGztqKqRR3YKHyCuXapnwXCfJOLwObEWj1vDLteA94pp\nqhzyapMI2vlA38nSxrdbidKfnUSsfx8bVsgcuyoCgYEAkxe50Tzw9uQWGWpZJYG1\ncxrFAuo0xO+ogzAm8h1Hn0SI7RyhokW2N7DbStO2Qd6hwMRyYB9H9n1tFoZT3zh0\nBTtPlqvGjufH6G+jD/adJzi10BGSAdoo6gWQBaIj++ImQxGc1dQc5sKXc5teLoI0\nlp4rWuIwoMvV7bgidh+NangCgYBhW7x1YgnSUZXoqBYwygJyI072QtdgQXl3k5uf\nADG7n2AFD+a83H8XTur2qxGn8pY/+bexdFv+DE5jBqFaUG2RgxN6E466+vWXT14s\nMUR3pvN8MPmMXxMvmP0xSg6u40qCMgfHdCqkfNNpJBWlAbIYW/W2PASi6DPd7OJb\nRRqtDQKBgA0uohsEiL/KZ8nfdXbra+XUl3Bd4mV9Ezg1Q8VE3d2r0Rk5v0zzakDx\nzY/boYYGr2susx1bwyZhH4qzM7gc3KJ2YMBX1IzBgE4nn3OJbGdv8ImZ/Sc7VcRb\n5t6dv3Vv4Y20N6l0e6QF6UVx/FuWRzE7dDzIvmhjYQdkWXNoKGdz\n-----END RSA PRIVATE KEY-----"
