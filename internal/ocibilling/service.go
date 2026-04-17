package ocibilling

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/oci"
	"ohoci/internal/store"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/usageapi"
)

const (
	defaultUsageBatchSize = 25
	defaultLagNotice      = "OCI Usage API data is delayed and the current day can remain incomplete until OCI finishes aggregation."
	defaultScopeNote      = "This report shows tenancy-scope OCI billed cost for the same window alongside OhoCI-tracked runner attribution. The detailed breakdown and gap review remain limited to tracked runner resources, so some non-runner OCI charges can stay outside attribution."
)

type UsageClient interface {
	RequestSummarizedUsages(ctx context.Context, request usageapi.RequestSummarizedUsagesRequest) (usageapi.RequestSummarizedUsagesResponse, error)
}

type Config struct {
	DefaultMode               string
	BillingTagNamespace       string
	ProviderResolver          oci.ProviderResolver
	InstancePrincipalProvider func() (common.ConfigurationProvider, error)
	UsageClientFactory        func(common.ConfigurationProvider) (UsageClient, error)
	UsageBatchSize            int
}

type Service struct {
	store                     *store.Store
	defaultMode               string
	billingTagNamespace       string
	providerResolver          oci.ProviderResolver
	instancePrincipalProvider func() (common.ConfigurationProvider, error)
	usageClientFactory        func(common.ConfigurationProvider) (UsageClient, error)
	usageBatchSize            int
}

type PolicyBreakdownRequest struct {
	WindowStart time.Time
	WindowEnd   time.Time
}

type PolicyCostBucket struct {
	TimeStart          time.Time `json:"timeStart"`
	TimeEnd            time.Time `json:"timeEnd"`
	TotalCost          float64   `json:"totalCost"`
	TotalUsageQuantity float64   `json:"totalUsageQuantity"`
	UsageUnits         []string  `json:"usageUnits,omitempty"`
}

type PolicyCostItem struct {
	PolicyID              int64              `json:"policyId"`
	PolicyLabel           string             `json:"policyLabel"`
	RepoOwner             string             `json:"repoOwner"`
	RepoName              string             `json:"repoName"`
	Currency              string             `json:"currency"`
	TotalCost             float64            `json:"totalCost"`
	TotalUsageQuantity    float64            `json:"totalUsageQuantity"`
	UsageUnits            []string           `json:"usageUnits,omitempty"`
	ResourceCount         int                `json:"resourceCount"`
	VerifiedResourceCount int                `json:"verifiedResourceCount"`
	FallbackResourceCount int                `json:"fallbackResourceCount"`
	TagOnlyResourceCount  int                `json:"tagOnlyResourceCount"`
	AttributionStatus     string             `json:"attributionStatus"`
	TimeSeries            []PolicyCostBucket `json:"timeSeries"`
}

type BillingIssue struct {
	ResourceID  string    `json:"resourceId"`
	PolicyID    *int64    `json:"policyId,omitempty"`
	PolicyLabel string    `json:"policyLabel,omitempty"`
	RepoOwner   string    `json:"repoOwner,omitempty"`
	RepoName    string    `json:"repoName,omitempty"`
	TagPolicyID string    `json:"tagPolicyId,omitempty"`
	Currency    string    `json:"currency,omitempty"`
	Cost        float64   `json:"cost"`
	TimeStart   time.Time `json:"timeStart"`
	TimeEnd     time.Time `json:"timeEnd"`
	Reason      string    `json:"reason"`
}

type PolicyBreakdown struct {
	WindowStart          time.Time        `json:"windowStart"`
	WindowEnd            time.Time        `json:"windowEnd"`
	Granularity          string           `json:"granularity"`
	GeneratedAt          time.Time        `json:"generatedAt"`
	SourceRegion         string           `json:"sourceRegion"`
	TagNamespace         string           `json:"tagNamespace,omitempty"`
	TagKey               string           `json:"tagKey,omitempty"`
	TagAttributionReady  bool             `json:"tagAttributionReady"`
	Currency             string           `json:"currency,omitempty"`
	OCIBilledCost        float64          `json:"ociBilledCost"`
	TotalCost            float64          `json:"totalCost"`
	MappedCost           float64          `json:"mappedCost"`
	TagVerifiedCost      float64          `json:"tagVerifiedCost"`
	ResourceFallbackCost float64          `json:"resourceFallbackCost"`
	TagOnlyCost          float64          `json:"tagOnlyCost"`
	UnmappedCost         float64          `json:"unmappedCost"`
	LagNotice            string           `json:"lagNotice"`
	ScopeNote            string           `json:"scopeNote"`
	Items                []PolicyCostItem `json:"items"`
	Issues               []BillingIssue   `json:"issues"`
}

func New(s *store.Store, cfg Config) (*Service, error) {
	if s == nil {
		return nil, fmt.Errorf("store is required")
	}
	service := &Service{
		store:               s,
		defaultMode:         strings.TrimSpace(cfg.DefaultMode),
		billingTagNamespace: strings.TrimSpace(cfg.BillingTagNamespace),
		providerResolver:    cfg.ProviderResolver,
		usageBatchSize:      cfg.UsageBatchSize,
	}
	if service.usageBatchSize <= 0 {
		service.usageBatchSize = defaultUsageBatchSize
	}
	if cfg.InstancePrincipalProvider != nil {
		service.instancePrincipalProvider = cfg.InstancePrincipalProvider
	} else {
		service.instancePrincipalProvider = auth.InstancePrincipalConfigurationProvider
	}
	if cfg.UsageClientFactory != nil {
		service.usageClientFactory = cfg.UsageClientFactory
	} else {
		service.usageClientFactory = func(provider common.ConfigurationProvider) (UsageClient, error) {
			return usageapi.NewUsageapiClientWithConfigurationProvider(provider)
		}
	}
	return service, nil
}

func (s *Service) PolicyBreakdown(ctx context.Context, input PolicyBreakdownRequest) (PolicyBreakdown, error) {
	windowStart := input.WindowStart.UTC()
	windowEnd := input.WindowEnd.UTC()
	if windowEnd.Before(windowStart) || windowEnd.Equal(windowStart) {
		return PolicyBreakdown{}, fmt.Errorf("windowEnd must be after windowStart")
	}

	report := PolicyBreakdown{
		WindowStart:         windowStart,
		WindowEnd:           windowEnd,
		Granularity:         "DAILY",
		GeneratedAt:         time.Now().UTC(),
		TagNamespace:        s.billingTagNamespace,
		TagKey:              oci.BillingDefinedTagKeyPolicyID,
		TagAttributionReady: strings.TrimSpace(s.billingTagNamespace) != "",
		LagNotice:           defaultLagNotice,
		ScopeNote:           defaultScopeNote,
	}

	runners, err := s.store.ListBillingRunners(ctx, windowStart, windowEnd)
	if err != nil {
		return PolicyBreakdown{}, err
	}
	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return PolicyBreakdown{}, err
	}
	policyLabels := map[int64]string{}
	for _, policy := range policies {
		policyLabels[policy.ID] = policyLabel(policy)
	}

	resourceIDs := make([]string, 0, len(runners))
	runnerByResourceID := make(map[string]store.Runner, len(runners))
	for _, runner := range runners {
		resourceID := strings.TrimSpace(runner.InstanceOCID)
		if resourceID == "" {
			continue
		}
		resourceIDs = append(resourceIDs, resourceID)
		runnerByResourceID[resourceID] = runner
	}
	scope, err := s.resolveUsageScope(ctx)
	if err != nil {
		return PolicyBreakdown{}, err
	}
	if scope.mode == "fake" {
		return s.fakePolicyBreakdown(report, runners, policyLabels), nil
	}
	report.SourceRegion = scope.sourceRegion
	report.OCIBilledCost, report.Currency, err = s.collectOCIBilledCost(ctx, scope.client, scope.tenancyOCID, report)
	if err != nil {
		return PolicyBreakdown{}, err
	}
	if len(resourceIDs) == 0 {
		return report, nil
	}

	rows, err := s.collectUsageRows(ctx, scope.client, scope.tenancyOCID, resourceIDs, report)
	if err != nil {
		return PolicyBreakdown{}, err
	}

	return buildPolicyBreakdown(report, rows, runnerByResourceID, policyLabels), nil
}

type usageRowQuery struct {
	GroupBy    []string
	GroupByTag []usageapi.Tag
	Filter     *usageapi.Filter
}

type usageScope struct {
	mode         string
	client       UsageClient
	tenancyOCID  string
	sourceRegion string
}

func (s *Service) collectOCIBilledCost(ctx context.Context, client UsageClient, tenancyOCID string, report PolicyBreakdown) (float64, string, error) {
	rows, err := s.requestUsageRows(ctx, client, tenancyOCID, report, usageRowQuery{})
	if err != nil {
		return 0, "", err
	}
	return usageRowsCost(rows), usageRowsCurrency(rows), nil
}

func (s *Service) collectUsageRows(ctx context.Context, client UsageClient, tenancyOCID string, resourceIDs []string, report PolicyBreakdown) ([]usageapi.UsageSummary, error) {
	if len(resourceIDs) == 0 {
		return nil, nil
	}
	allRows := []usageapi.UsageSummary{}
	for start := 0; start < len(resourceIDs); start += s.usageBatchSize {
		end := start + s.usageBatchSize
		if end > len(resourceIDs) {
			end = len(resourceIDs)
		}
		batch := resourceIDs[start:end]
		rows, err := s.requestUsageRows(ctx, client, tenancyOCID, report, usageRowQuery{
			GroupBy:    []string{"resourceId"},
			GroupByTag: buildGroupByTag(report.TagNamespace),
			Filter:     buildResourceIDFilter(batch),
		})
		if err != nil {
			return nil, err
		}
		allRows = append(allRows, rows...)
	}
	return allRows, nil
}

func (s *Service) resolveUsageScope(ctx context.Context) (usageScope, error) {
	mode, provider, err := s.resolveProvider(ctx)
	if err != nil {
		return usageScope{}, err
	}
	if mode == "fake" {
		return usageScope{mode: mode}, nil
	}

	sourceRegion, err := provider.Region()
	if err != nil {
		return usageScope{}, fmt.Errorf("resolve source region: %w", err)
	}
	client, err := s.usageClientFactory(provider)
	if err != nil {
		return usageScope{}, fmt.Errorf("create OCI usage client: %w", err)
	}
	tenancyOCID, err := provider.TenancyOCID()
	if err != nil {
		return usageScope{}, fmt.Errorf("resolve tenancy: %w", err)
	}

	return usageScope{
		mode:         mode,
		client:       client,
		tenancyOCID:  strings.TrimSpace(tenancyOCID),
		sourceRegion: strings.TrimSpace(sourceRegion),
	}, nil
}

func (s *Service) requestUsageRows(ctx context.Context, client UsageClient, tenancyOCID string, report PolicyBreakdown, query usageRowQuery) ([]usageapi.UsageSummary, error) {
	request := usageapi.RequestSummarizedUsagesRequest{
		RequestSummarizedUsagesDetails: usageapi.RequestSummarizedUsagesDetails{
			TenantId:         common.String(tenancyOCID),
			TimeUsageStarted: &common.SDKTime{Time: report.WindowStart},
			TimeUsageEnded:   &common.SDKTime{Time: report.WindowEnd},
			Granularity:      usageapi.RequestSummarizedUsagesDetailsGranularityDaily,
			QueryType:        usageapi.RequestSummarizedUsagesDetailsQueryTypeCost,
			GroupBy:          query.GroupBy,
			GroupByTag:       query.GroupByTag,
			Filter:           query.Filter,
		},
		Limit: common.Int(1000),
	}
	rows := []usageapi.UsageSummary{}
	for {
		response, err := client.RequestSummarizedUsages(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("request OCI usage rows: %w", err)
		}
		rows = append(rows, response.Items...)
		if response.OpcNextPage == nil || strings.TrimSpace(*response.OpcNextPage) == "" {
			break
		}
		request.Page = response.OpcNextPage
	}
	return rows, nil
}

func buildPolicyBreakdown(base PolicyBreakdown, rows []usageapi.UsageSummary, runnerByResourceID map[string]store.Runner, policyLabels map[int64]string) PolicyBreakdown {
	items := map[string]*policyCostAccumulator{}
	unmapped := []BillingIssue{}

	for _, row := range rows {
		cost := usageCost(row)
		usageQuantity := usageQuantity(row)
		currency := strings.TrimSpace(valueOrEmpty(row.Currency))
		resourceID := strings.TrimSpace(valueOrEmpty(row.ResourceId))
		tagPolicyID := tagValueForUsage(row.Tags, base.TagNamespace, base.TagKey)
		timeStart := sdkTimeValue(row.TimeUsageStarted)
		timeEnd := sdkTimeValue(row.TimeUsageEnded)
		unit := strings.TrimSpace(valueOrEmpty(row.Unit))

		base.TotalCost += cost
		if base.Currency == "" && currency != "" {
			base.Currency = currency
		}

		runner, hasRunner := runnerByResourceID[resourceID]
		tagPolicyIDValue, tagPolicyIDOK := parseInt64TagValue(tagPolicyID)

		switch {
		case hasRunner && tagPolicyIDOK && tagPolicyIDValue == runner.PolicyID:
			key := policyRepoKey(runner.PolicyID, runner.RepoOwner, runner.RepoName)
			accumulator := ensurePolicyAccumulator(items, key, runner.PolicyID, policyLabels[runner.PolicyID], runner.RepoOwner, runner.RepoName, currency)
			accumulator.add(resourceID, cost, usageQuantity, unit, timeStart, timeEnd, attributionTagVerified)
			base.MappedCost += cost
			base.TagVerifiedCost += cost
		case hasRunner:
			key := policyRepoKey(runner.PolicyID, runner.RepoOwner, runner.RepoName)
			accumulator := ensurePolicyAccumulator(items, key, runner.PolicyID, policyLabels[runner.PolicyID], runner.RepoOwner, runner.RepoName, currency)
			accumulator.add(resourceID, cost, usageQuantity, unit, timeStart, timeEnd, attributionResourceFallback)
			base.MappedCost += cost
			base.ResourceFallbackCost += cost
			if base.TagAttributionReady {
				unmapped = append(unmapped, BillingIssue{
					ResourceID:  resourceID,
					PolicyID:    int64Pointer(runner.PolicyID),
					PolicyLabel: accumulator.policyLabel,
					RepoOwner:   runner.RepoOwner,
					RepoName:    runner.RepoName,
					TagPolicyID: tagPolicyID,
					Currency:    currency,
					Cost:        cost,
					TimeStart:   timeStart,
					TimeEnd:     timeEnd,
					Reason:      fallbackReason(tagPolicyID, runner.PolicyID),
				})
			}
		case tagPolicyIDOK && policyLabels[tagPolicyIDValue] != "":
			key := policyRepoKey(tagPolicyIDValue, "", "")
			accumulator := ensurePolicyAccumulator(items, key, tagPolicyIDValue, policyLabels[tagPolicyIDValue], "", "", currency)
			accumulator.add(resourceID, cost, usageQuantity, unit, timeStart, timeEnd, attributionTagOnly)
			base.MappedCost += cost
			base.TagOnlyCost += cost
			unmapped = append(unmapped, BillingIssue{
				ResourceID:  resourceID,
				PolicyID:    int64Pointer(tagPolicyIDValue),
				PolicyLabel: accumulator.policyLabel,
				TagPolicyID: tagPolicyID,
				Currency:    currency,
				Cost:        cost,
				TimeStart:   timeStart,
				TimeEnd:     timeEnd,
				Reason:      "tag_only_without_tracked_runner",
			})
		case tagPolicyIDOK:
			base.UnmappedCost += cost
			unmapped = append(unmapped, BillingIssue{
				ResourceID:  resourceID,
				PolicyID:    int64Pointer(tagPolicyIDValue),
				TagPolicyID: tagPolicyID,
				Currency:    currency,
				Cost:        cost,
				TimeStart:   timeStart,
				TimeEnd:     timeEnd,
				Reason:      "stale_policy_tag_without_tracked_runner",
			})
		default:
			base.UnmappedCost += cost
			unmapped = append(unmapped, BillingIssue{
				ResourceID:  resourceID,
				TagPolicyID: tagPolicyID,
				Currency:    currency,
				Cost:        cost,
				TimeStart:   timeStart,
				TimeEnd:     timeEnd,
				Reason:      "unmapped_resource_usage",
			})
		}
	}

	base.Items = make([]PolicyCostItem, 0, len(items))
	for _, accumulator := range items {
		base.Items = append(base.Items, accumulator.item())
	}
	slices.SortFunc(base.Items, func(a, b PolicyCostItem) int {
		switch {
		case a.TotalCost > b.TotalCost:
			return -1
		case a.TotalCost < b.TotalCost:
			return 1
		}
		return strings.Compare(strings.ToLower(a.PolicyLabel+" "+a.RepoOwner+"/"+a.RepoName), strings.ToLower(b.PolicyLabel+" "+b.RepoOwner+"/"+b.RepoName))
	})

	slices.SortFunc(unmapped, func(a, b BillingIssue) int {
		switch {
		case a.Cost > b.Cost:
			return -1
		case a.Cost < b.Cost:
			return 1
		}
		return strings.Compare(strings.ToLower(a.ResourceID), strings.ToLower(b.ResourceID))
	})
	base.Issues = unmapped
	return base
}

func (s *Service) fakePolicyBreakdown(base PolicyBreakdown, runners []store.Runner, policyLabels map[int64]string) PolicyBreakdown {
	items := map[string]*policyCostAccumulator{}
	costPerRunner := 0.24
	for _, runner := range runners {
		key := policyRepoKey(runner.PolicyID, runner.RepoOwner, runner.RepoName)
		accumulator := ensurePolicyAccumulator(items, key, runner.PolicyID, policyLabels[runner.PolicyID], runner.RepoOwner, runner.RepoName, "USD")
		status := attributionResourceFallback
		if base.TagAttributionReady {
			status = attributionTagVerified
			base.TagVerifiedCost += costPerRunner
		} else {
			base.ResourceFallbackCost += costPerRunner
		}
		base.TotalCost += costPerRunner
		base.MappedCost += costPerRunner
		base.Currency = "USD"
		accumulator.add(runner.InstanceOCID, costPerRunner, 1, "OCPU Hours", base.WindowStart, base.WindowEnd, status)
	}
	base.OCIBilledCost = base.TotalCost
	base.Items = make([]PolicyCostItem, 0, len(items))
	for _, accumulator := range items {
		base.Items = append(base.Items, accumulator.item())
	}
	slices.SortFunc(base.Items, func(a, b PolicyCostItem) int {
		if a.TotalCost > b.TotalCost {
			return -1
		}
		if a.TotalCost < b.TotalCost {
			return 1
		}
		return strings.Compare(strings.ToLower(a.PolicyLabel), strings.ToLower(b.PolicyLabel))
	})
	base.SourceRegion = "fake-region-1"
	return base
}

func (s *Service) resolveProvider(ctx context.Context) (string, common.ConfigurationProvider, error) {
	if s.providerResolver != nil {
		provider, ok, err := s.providerResolver.ResolveProvider(ctx)
		if err != nil {
			return "", nil, err
		}
		if ok {
			return "api_key", provider, nil
		}
	}
	if strings.EqualFold(s.defaultMode, "fake") {
		return "fake", nil, nil
	}
	provider, err := s.instancePrincipalProvider()
	if err != nil {
		return "", nil, err
	}
	return "instance_principal", provider, nil
}

func buildGroupByTag(namespace string) []usageapi.Tag {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil
	}
	return []usageapi.Tag{{
		Namespace: common.String(namespace),
		Key:       common.String(oci.BillingDefinedTagKeyPolicyID),
	}}
}

func buildResourceIDFilter(resourceIDs []string) *usageapi.Filter {
	resourceIDs = normalizeResourceIDs(resourceIDs)
	if len(resourceIDs) == 0 {
		return nil
	}
	if len(resourceIDs) == 1 {
		return &usageapi.Filter{
			Dimensions: []usageapi.Dimension{{
				Key:   common.String("resourceId"),
				Value: common.String(resourceIDs[0]),
			}},
		}
	}
	filters := make([]usageapi.Filter, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		filters = append(filters, usageapi.Filter{
			Dimensions: []usageapi.Dimension{{
				Key:   common.String("resourceId"),
				Value: common.String(resourceID),
			}},
		})
	}
	return &usageapi.Filter{
		Operator: usageapi.FilterOperatorOr,
		Filters:  filters,
	}
}

func normalizeResourceIDs(resourceIDs []string) []string {
	out := make([]string, 0, len(resourceIDs))
	seen := map[string]struct{}{}
	for _, resourceID := range resourceIDs {
		resourceID = strings.TrimSpace(resourceID)
		if resourceID == "" {
			continue
		}
		if _, exists := seen[resourceID]; exists {
			continue
		}
		seen[resourceID] = struct{}{}
		out = append(out, resourceID)
	}
	slices.Sort(out)
	return out
}

func policyLabel(policy store.Policy) string {
	if strings.TrimSpace(policy.Label) != "" {
		return strings.TrimSpace(policy.Label)
	}
	if len(policy.Labels) > 0 {
		return strings.Join(policy.Labels, "-")
	}
	return fmt.Sprintf("Policy #%d", policy.ID)
}

func parseInt64TagValue(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func tagValueForUsage(tags []usageapi.Tag, namespace, key string) string {
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" || key == "" {
		return ""
	}
	for _, tag := range tags {
		if !strings.EqualFold(valueOrEmpty(tag.Namespace), namespace) {
			continue
		}
		if !strings.EqualFold(valueOrEmpty(tag.Key), key) {
			continue
		}
		return strings.TrimSpace(valueOrEmpty(tag.Value))
	}
	return ""
}

func fallbackReason(tagPolicyID string, runnerPolicyID int64) string {
	tagPolicyID = strings.TrimSpace(tagPolicyID)
	switch {
	case tagPolicyID == "":
		return "missing_policy_tag_resource_fallback"
	case !strings.EqualFold(tagPolicyID, strconv.FormatInt(runnerPolicyID, 10)):
		return "tag_mismatch_resource_fallback"
	default:
		return "resource_fallback"
	}
}

func usageCost(item usageapi.UsageSummary) float64 {
	if item.ComputedAmount != nil {
		return float64(*item.ComputedAmount)
	}
	if item.AttributedCost != nil {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(*item.AttributedCost), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func usageRowsCost(items []usageapi.UsageSummary) float64 {
	total := 0.0
	for _, item := range items {
		total += usageCost(item)
	}
	return total
}

func usageRowsCurrency(items []usageapi.UsageSummary) string {
	for _, item := range items {
		currency := strings.TrimSpace(valueOrEmpty(item.Currency))
		if currency != "" {
			return currency
		}
	}
	return ""
}

func usageQuantity(item usageapi.UsageSummary) float64 {
	if item.ComputedQuantity == nil {
		return 0
	}
	return float64(*item.ComputedQuantity)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sdkTimeValue(value *common.SDKTime) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.Time.UTC()
}

func int64Pointer(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	copy := value
	return &copy
}

const (
	attributionTagVerified      = "tag_verified"
	attributionResourceFallback = "resource_fallback"
	attributionTagOnly          = "tag_only"
)

type policyCostAccumulator struct {
	policyID           int64
	policyLabel        string
	repoOwner          string
	repoName           string
	currency           string
	totalCost          float64
	totalUsageQuantity float64
	usageUnits         map[string]struct{}
	resourceStatuses   map[string]string
	buckets            map[string]*policyCostBucketAccumulator
}

type policyCostBucketAccumulator struct {
	timeStart          time.Time
	timeEnd            time.Time
	totalCost          float64
	totalUsageQuantity float64
	usageUnits         map[string]struct{}
}

func ensurePolicyAccumulator(items map[string]*policyCostAccumulator, key string, policyID int64, policyLabel, repoOwner, repoName, currency string) *policyCostAccumulator {
	if existing, ok := items[key]; ok {
		if existing.currency == "" && currency != "" {
			existing.currency = currency
		}
		return existing
	}
	label := strings.TrimSpace(policyLabel)
	if label == "" {
		label = fmt.Sprintf("Policy #%d", policyID)
	}
	accumulator := &policyCostAccumulator{
		policyID:         policyID,
		policyLabel:      label,
		repoOwner:        strings.TrimSpace(repoOwner),
		repoName:         strings.TrimSpace(repoName),
		currency:         strings.TrimSpace(currency),
		usageUnits:       map[string]struct{}{},
		resourceStatuses: map[string]string{},
		buckets:          map[string]*policyCostBucketAccumulator{},
	}
	items[key] = accumulator
	return accumulator
}

func (a *policyCostAccumulator) add(resourceID string, cost, usageQuantity float64, unit string, timeStart, timeEnd time.Time, status string) {
	a.totalCost += cost
	a.totalUsageQuantity += usageQuantity
	if unit != "" {
		a.usageUnits[unit] = struct{}{}
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID != "" {
		a.resourceStatuses[resourceID] = mergeAttributionStatus(a.resourceStatuses[resourceID], status)
	}
	bucketKey := timeStart.UTC().Format(time.RFC3339)
	bucket := a.buckets[bucketKey]
	if bucket == nil {
		bucket = &policyCostBucketAccumulator{
			timeStart:  timeStart.UTC(),
			timeEnd:    timeEnd.UTC(),
			usageUnits: map[string]struct{}{},
		}
		a.buckets[bucketKey] = bucket
	}
	bucket.totalCost += cost
	bucket.totalUsageQuantity += usageQuantity
	if unit != "" {
		bucket.usageUnits[unit] = struct{}{}
	}
}

func (a *policyCostAccumulator) item() PolicyCostItem {
	item := PolicyCostItem{
		PolicyID:           a.policyID,
		PolicyLabel:        a.policyLabel,
		RepoOwner:          a.repoOwner,
		RepoName:           a.repoName,
		Currency:           a.currency,
		TotalCost:          a.totalCost,
		TotalUsageQuantity: a.totalUsageQuantity,
		UsageUnits:         sortedSetValues(a.usageUnits),
		ResourceCount:      len(a.resourceStatuses),
		TimeSeries:         make([]PolicyCostBucket, 0, len(a.buckets)),
	}
	overallStatus := ""
	for _, status := range a.resourceStatuses {
		switch status {
		case attributionTagVerified:
			item.VerifiedResourceCount += 1
		case attributionResourceFallback:
			item.FallbackResourceCount += 1
		case attributionTagOnly:
			item.TagOnlyResourceCount += 1
		}
		overallStatus = mergeAttributionStatus(overallStatus, status)
	}
	item.AttributionStatus = overallStatus
	for _, bucket := range a.buckets {
		item.TimeSeries = append(item.TimeSeries, PolicyCostBucket{
			TimeStart:          bucket.timeStart,
			TimeEnd:            bucket.timeEnd,
			TotalCost:          bucket.totalCost,
			TotalUsageQuantity: bucket.totalUsageQuantity,
			UsageUnits:         sortedSetValues(bucket.usageUnits),
		})
	}
	slices.SortFunc(item.TimeSeries, func(left, right PolicyCostBucket) int {
		switch {
		case left.TimeStart.Before(right.TimeStart):
			return -1
		case left.TimeStart.After(right.TimeStart):
			return 1
		}
		return 0
	})
	return item
}

func mergeAttributionStatus(current, next string) string {
	switch {
	case current == "":
		return next
	case current == next:
		return current
	default:
		return "mixed"
	}
}

func sortedSetValues(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func policyRepoKey(policyID int64, repoOwner, repoName string) string {
	return fmt.Sprintf("%d:%s/%s", policyID, strings.ToLower(strings.TrimSpace(repoOwner)), strings.ToLower(strings.TrimSpace(repoName)))
}
