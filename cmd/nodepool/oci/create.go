package oci

import (
	"context"
	"fmt"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/hypershift/cmd/nodepool/core"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type RawOCIPlatformCreateOptions struct {
	InstanceShape      string
	OCPUs              int64
	MemoryInGBs        int64
	AvailabilityDomain string
	SubnetID           string
	BootVolumeSize     int64
}

func DefaultOptions() *RawOCIPlatformCreateOptions {
	return &RawOCIPlatformCreateOptions{
		InstanceShape: "VM.Standard.E4.Flex",
		OCPUs:         4,
		MemoryInGBs:   16,
	}
}

func BindOptions(opts *RawOCIPlatformCreateOptions, flags *pflag.FlagSet) {
	flags.StringVar(&opts.InstanceShape, "instance-shape", opts.InstanceShape, "The OCI instance shape for node instances (e.g., VM.Standard.E4.Flex)")
	flags.Int64Var(&opts.OCPUs, "ocpus", opts.OCPUs, "The number of OCPUs for flexible instance shapes")
	flags.Int64Var(&opts.MemoryInGBs, "memory-in-gbs", opts.MemoryInGBs, "The amount of memory in GiB for flexible instance shapes")
	flags.StringVar(&opts.AvailabilityDomain, "availability-domain", opts.AvailabilityDomain, "The OCI availability domain for node instances (required)")
	flags.StringVar(&opts.SubnetID, "subnet-id", opts.SubnetID, "The OCID of the subnet for node instances (required)")
	flags.Int64Var(&opts.BootVolumeSize, "boot-volume-size", opts.BootVolumeSize, "The boot volume size in GiB for node instances (optional, 0 uses default)")
}

func NewCreateCommand(coreOpts *core.CreateNodePoolOptions) *cobra.Command {
	platformOpts := DefaultOptions()

	cmd := &cobra.Command{
		Use:          "oci",
		Short:        "Creates basic functional NodePool resources for OCI platform",
		SilenceUsage: true,
	}

	BindOptions(platformOpts, cmd.Flags())
	cmd.RunE = coreOpts.CreateRunFunc(platformOpts)

	return cmd
}

func (o *RawOCIPlatformCreateOptions) UpdateNodePool(_ context.Context, nodePool *hyperv1.NodePool, _ *hyperv1.HostedCluster, _ crclient.Client) error {
	if o.InstanceShape == "" {
		return fmt.Errorf("instance shape is required")
	}
	if o.AvailabilityDomain == "" {
		return fmt.Errorf("availability domain is required")
	}
	if o.SubnetID == "" {
		return fmt.Errorf("subnet ID is required")
	}

	nodePool.Spec.Platform.OCI = &hyperv1.OCINodePoolPlatform{
		InstanceShape:      o.InstanceShape,
		AvailabilityDomain: o.AvailabilityDomain,
		SubnetID:           o.SubnetID,
	}
	if o.OCPUs > 0 && o.MemoryInGBs > 0 {
		nodePool.Spec.Platform.OCI.InstanceShapeConfig = &hyperv1.OCIInstanceShapeConfig{
			OCPUs:       o.OCPUs,
			MemoryInGBs: o.MemoryInGBs,
		}
	}
	if o.BootVolumeSize > 0 {
		nodePool.Spec.Platform.OCI.BootVolumeSize = &o.BootVolumeSize
	}
	return nil
}

func (o *RawOCIPlatformCreateOptions) Type() hyperv1.PlatformType {
	return hyperv1.OCIPlatform
}
