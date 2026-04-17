package oci

import (
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestBuildSubnetCandidateRecommendsPrivateSubnetWithNAT(t *testing.T) {
	subnet := core.Subnet{
		Id:                     common.String("ocid1.subnet.oc1..private"),
		DisplayName:            common.String("runner-private"),
		CidrBlock:              common.String("10.0.1.0/24"),
		ProhibitPublicIpOnVnic: common.Bool(true),
	}
	routeTable := core.RouteTable{
		RouteRules: []core.RouteRule{
			{
				Destination:     common.String("0.0.0.0/0"),
				NetworkEntityId: common.String("ocid1.natgateway.oc1..nat"),
			},
		},
	}

	candidate := buildSubnetCandidate(subnet, routeTable, "ocid1.subnet.oc1..private")
	if !candidate.IsRecommended {
		t.Fatalf("expected subnet to be recommended")
	}
	if !candidate.HasDefaultRouteToNAT {
		t.Fatalf("expected NAT route to be detected")
	}
	if !candidate.IsCurrentDefault {
		t.Fatalf("expected subnet to be marked as default")
	}
}

func TestBuildSubnetCandidateRejectsPublicSubnet(t *testing.T) {
	subnet := core.Subnet{
		Id:                     common.String("ocid1.subnet.oc1..public"),
		DisplayName:            common.String("runner-public"),
		CidrBlock:              common.String("10.0.2.0/24"),
		ProhibitPublicIpOnVnic: common.Bool(false),
	}
	routeTable := core.RouteTable{
		RouteRules: []core.RouteRule{
			{
				Destination:     common.String("0.0.0.0/0"),
				NetworkEntityId: common.String("ocid1.internetgateway.oc1..igw"),
			},
		},
	}

	candidate := buildSubnetCandidate(subnet, routeTable, "")
	if candidate.IsRecommended {
		t.Fatalf("expected public subnet to be rejected")
	}
	if !candidate.HasDefaultRouteToInternet {
		t.Fatalf("expected internet gateway route to be detected")
	}
}
