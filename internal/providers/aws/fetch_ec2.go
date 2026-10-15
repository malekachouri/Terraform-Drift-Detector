package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/malekachouri/terraform-drift-detector/internal/awsclient"
	"github.com/malekachouri/terraform-drift-detector/internal/canonical"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

func fetchInstances(ctx context.Context, c *awsclient.Clients, region string) ([]model.Resource, error) {
	var resources []model.Resource
	pager := ec2.NewDescribeInstancesPaginator(c.EC2, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			// terminated instances are gone as far as drift is concerned
			Name:   aws.String("instance-state-name"),
			Values: []string{"pending", "running", "stopping", "stopped"},
		}},
	})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return resources, fmt.Errorf("describe instances: %w", err)
		}
		for _, res := range out.Reservations {
			for _, inst := range res.Instances {
				id := aws.ToString(inst.InstanceId)
				sgIDs := make([]string, 0, len(inst.SecurityGroups))
				for _, sg := range inst.SecurityGroups {
					sgIDs = append(sgIDs, aws.ToString(sg.GroupId))
				}
				tags := ec2TagMap(inst.Tags)
				attrs := map[string]any{
					"instance_type":          string(inst.InstanceType),
					"ami":                    aws.ToString(inst.ImageId),
					"subnet_id":              aws.ToString(inst.SubnetId),
					"vpc_security_group_ids": canonical.SortedStrings(sgIDs),
					"monitoring":             inst.Monitoring != nil && inst.Monitoring.State == ec2types.MonitoringStateEnabled,
				}
				resources = append(resources, baseResource("aws_instance", id, tags["Name"], region, attrs, tags))
			}
		}
	}
	return resources, nil
}

func fetchVPCs(ctx context.Context, c *awsclient.Clients, region string) ([]model.Resource, error) {
	var resources []model.Resource
	pager := ec2.NewDescribeVpcsPaginator(c.EC2, &ec2.DescribeVpcsInput{})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return resources, fmt.Errorf("describe vpcs: %w", err)
		}
		for _, vpc := range out.Vpcs {
			if aws.ToBool(vpc.IsDefault) {
				continue // AWS-created default VPC, never managed by Terraform here
			}
			id := aws.ToString(vpc.VpcId)
			hostnames, err := vpcAttr(ctx, c.EC2, id, ec2types.VpcAttributeNameEnableDnsHostnames)
			if err != nil {
				return resources, err
			}
			support, err := vpcAttr(ctx, c.EC2, id, ec2types.VpcAttributeNameEnableDnsSupport)
			if err != nil {
				return resources, err
			}
			tags := ec2TagMap(vpc.Tags)
			attrs := map[string]any{
				"cidr_block":           aws.ToString(vpc.CidrBlock),
				"enable_dns_hostnames": hostnames,
				"enable_dns_support":   support,
				"instance_tenancy":     string(vpc.InstanceTenancy),
			}
			resources = append(resources, baseResource("aws_vpc", id, tags["Name"], region, attrs, tags))
		}
	}
	return resources, nil
}

func vpcAttr(ctx context.Context, client *ec2.Client, vpcID string, attr ec2types.VpcAttributeName) (bool, error) {
	out, err := client.DescribeVpcAttribute(ctx, &ec2.DescribeVpcAttributeInput{
		VpcId:     aws.String(vpcID),
		Attribute: attr,
	})
	if err != nil {
		return false, fmt.Errorf("describe vpc attribute %s on %s: %w", attr, vpcID, err)
	}
	switch attr {
	case ec2types.VpcAttributeNameEnableDnsHostnames:
		return out.EnableDnsHostnames != nil && aws.ToBool(out.EnableDnsHostnames.Value), nil
	default:
		return out.EnableDnsSupport != nil && aws.ToBool(out.EnableDnsSupport.Value), nil
	}
}

func fetchSubnets(ctx context.Context, c *awsclient.Clients, region string) ([]model.Resource, error) {
	var resources []model.Resource
	pager := ec2.NewDescribeSubnetsPaginator(c.EC2, &ec2.DescribeSubnetsInput{})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return resources, fmt.Errorf("describe subnets: %w", err)
		}
		for _, subnet := range out.Subnets {
			if aws.ToBool(subnet.DefaultForAz) {
				continue
			}
			id := aws.ToString(subnet.SubnetId)
			tags := ec2TagMap(subnet.Tags)
			attrs := map[string]any{
				"cidr_block":              aws.ToString(subnet.CidrBlock),
				"vpc_id":                  aws.ToString(subnet.VpcId),
				"map_public_ip_on_launch": aws.ToBool(subnet.MapPublicIpOnLaunch),
				"availability_zone":       aws.ToString(subnet.AvailabilityZone),
			}
			resources = append(resources, baseResource("aws_subnet", id, tags["Name"], region, attrs, tags))
		}
	}
	return resources, nil
}

func fetchSecurityGroups(ctx context.Context, c *awsclient.Clients, region string) ([]model.Resource, error) {
	var resources []model.Resource
	pager := ec2.NewDescribeSecurityGroupsPaginator(c.EC2, &ec2.DescribeSecurityGroupsInput{})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return resources, fmt.Errorf("describe security groups: %w", err)
		}
		for _, sg := range out.SecurityGroups {
			if aws.ToString(sg.GroupName) == "default" {
				continue // every VPC gets one automatically
			}
			id := aws.ToString(sg.GroupId)
			tags := ec2TagMap(sg.Tags)
			name := tags["Name"]
			if name == "" {
				name = aws.ToString(sg.GroupName)
			}
			attrs := map[string]any{
				"description": aws.ToString(sg.Description),
				"vpc_id":      aws.ToString(sg.VpcId),
				"ingress":     normalizeRules(sg.IpPermissions, id),
				"egress":      normalizeRules(sg.IpPermissionsEgress, id),
			}
			resources = append(resources, baseResource("aws_security_group", id, name, region, attrs, tags))
		}
	}
	return resources, nil
}

func ec2TagMap(tags []ec2types.Tag) map[string]string {
	out := make(map[string]string)
	for _, t := range tags {
		out[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return out
}

func normalizeRules(perms []ec2types.IpPermission, selfID string) []string {
	var out []string
	for _, p := range perms {
		var sources []string
		for _, r := range p.IpRanges {
			sources = append(sources, aws.ToString(r.CidrIp))
		}
		for _, r := range p.Ipv6Ranges {
			sources = append(sources, aws.ToString(r.CidrIpv6))
		}
		for _, r := range p.PrefixListIds {
			sources = append(sources, aws.ToString(r.PrefixListId))
		}
		for _, r := range p.UserIdGroupPairs {
			id := aws.ToString(r.GroupId)
			if id == selfID {
				id = "self"
			}
			sources = append(sources, id)
		}
		out = append(out, canonical.SGRule(aws.ToString(p.IpProtocol),
			int64(aws.ToInt32(p.FromPort)), int64(aws.ToInt32(p.ToPort)), sources)...)
	}
	return canonical.SortedStrings(out)
}
