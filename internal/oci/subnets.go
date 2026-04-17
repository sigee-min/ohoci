package oci

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (c *OCIController) ListSubnetCandidates(ctx context.Context) ([]SubnetCandidate, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return nil, err
	}
	if mode == "fake" {
		return c.fake.ListSubnetCandidates(ctx)
	}
	runtime, err := c.runtimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	networkClient, err := c.networkClientFactory(provider)
	if err != nil {
		return nil, err
	}
	anchorSubnetID := strings.TrimSpace(runtime.SubnetID)
	if anchorSubnetID == "" {
		return nil, fmt.Errorf("default OCI subnet is required for subnet discovery")
	}
	anchor, err := networkClient.GetSubnet(ctx, core.GetSubnetRequest{
		SubnetId: common.String(anchorSubnetID),
	})
	if err != nil {
		return nil, err
	}
	vcnID := valueOrEmpty(anchor.Subnet.VcnId)
	compartmentID := valueOrEmpty(anchor.Subnet.CompartmentId)
	if vcnID == "" || compartmentID == "" {
		return nil, fmt.Errorf("anchor subnet is missing VCN or compartment metadata")
	}

	subnets := []core.Subnet{}
	request := core.ListSubnetsRequest{
		CompartmentId:  common.String(compartmentID),
		VcnId:          common.String(vcnID),
		LifecycleState: core.SubnetLifecycleStateAvailable,
		SortBy:         core.ListSubnetsSortByDisplayname,
		SortOrder:      core.ListSubnetsSortOrderAsc,
		Limit:          common.Int(100),
	}
	for {
		response, err := networkClient.ListSubnets(ctx, request)
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, response.Items...)
		if response.OpcNextPage == nil || strings.TrimSpace(*response.OpcNextPage) == "" {
			break
		}
		request.Page = response.OpcNextPage
	}

	routeTables := map[string]core.RouteTable{}
	candidates := make([]SubnetCandidate, 0, len(subnets))
	for _, subnet := range subnets {
		routeTable, err := c.routeTableForSubnet(ctx, networkClient, subnet, routeTables)
		if err != nil {
			return nil, err
		}
		candidate := buildSubnetCandidate(subnet, routeTable, anchorSubnetID)
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, compareSubnetCandidates)
	return candidates, nil
}

func (c *OCIController) routeTableForSubnet(ctx context.Context, networkClient core.VirtualNetworkClient, subnet core.Subnet, cache map[string]core.RouteTable) (core.RouteTable, error) {
	routeTableID := valueOrEmpty(subnet.RouteTableId)
	if routeTableID == "" {
		return core.RouteTable{}, nil
	}
	if routeTable, ok := cache[routeTableID]; ok {
		return routeTable, nil
	}
	response, err := networkClient.GetRouteTable(ctx, core.GetRouteTableRequest{
		RtId: common.String(routeTableID),
	})
	if err != nil {
		return core.RouteTable{}, err
	}
	cache[routeTableID] = response.RouteTable
	return response.RouteTable, nil
}

func buildSubnetCandidate(subnet core.Subnet, routeTable core.RouteTable, currentDefaultSubnetID string) SubnetCandidate {
	privateSubnet := pointerBool(subnet.ProhibitPublicIpOnVnic)
	hasNAT := hasDefaultRouteToPrefix(routeTable.RouteRules, "ocid1.natgateway")
	hasInternet := hasDefaultRouteToPrefix(routeTable.RouteRules, "ocid1.internetgateway")
	recommendation := subnetRecommendation(privateSubnet, hasNAT, hasInternet)
	return SubnetCandidate{
		ID:                        valueOrEmpty(subnet.Id),
		DisplayName:               valueOrEmpty(subnet.DisplayName),
		CidrBlock:                 valueOrEmpty(subnet.CidrBlock),
		AvailabilityDomain:        valueOrEmpty(subnet.AvailabilityDomain),
		ProhibitPublicIPOnVnic:    privateSubnet,
		HasDefaultRouteToNAT:      hasNAT,
		HasDefaultRouteToInternet: hasInternet,
		IsCurrentDefault:          strings.EqualFold(valueOrEmpty(subnet.Id), strings.TrimSpace(currentDefaultSubnetID)),
		IsRecommended:             privateSubnet && hasNAT && !hasInternet,
		Recommendation:            recommendation,
	}
}

func subnetRecommendation(privateSubnet, hasNAT, hasInternet bool) string {
	switch {
	case privateSubnet && hasNAT && !hasInternet:
		return "Private subnet with default NAT route"
	case !privateSubnet && hasInternet:
		return "Public subnet with internet gateway route"
	case privateSubnet && !hasNAT:
		return "Private subnet without default NAT route"
	case hasNAT:
		return "Default NAT route detected"
	default:
		return "No default NAT route detected"
	}
}

func hasDefaultRouteToPrefix(rules []core.RouteRule, prefix string) bool {
	for _, rule := range rules {
		destination := strings.TrimSpace(valueOrEmpty(rule.Destination))
		if destination == "" {
			destination = strings.TrimSpace(valueOrEmpty(rule.CidrBlock))
		}
		if destination != "0.0.0.0/0" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(valueOrEmpty(rule.NetworkEntityId)), prefix) {
			return true
		}
	}
	return false
}

func compareSubnetCandidates(a, b SubnetCandidate) int {
	aScore := subnetCandidateScore(a)
	bScore := subnetCandidateScore(b)
	switch {
	case aScore > bScore:
		return -1
	case aScore < bScore:
		return 1
	}
	return strings.Compare(strings.ToLower(a.DisplayName+" "+a.ID), strings.ToLower(b.DisplayName+" "+b.ID))
}

func subnetCandidateScore(candidate SubnetCandidate) int {
	score := 0
	if candidate.IsRecommended {
		score += 10
	}
	if candidate.IsCurrentDefault {
		score += 5
	}
	if candidate.ProhibitPublicIPOnVnic {
		score += 2
	}
	if candidate.HasDefaultRouteToNAT {
		score += 2
	}
	if candidate.HasDefaultRouteToInternet {
		score -= 4
	}
	return score
}

func pointerBool(value *bool) bool {
	return value != nil && *value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
