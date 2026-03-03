package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/hypershift/cmd/cluster/core"
	"github.com/openshift/hypershift/cmd/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func DefaultOptions() *RawCreateOptions {
	return &RawCreateOptions{
		NodePoolOpts: &OCINodePoolCreateOptions{},
	}
}

func BindOptions(opts *RawCreateOptions, flags *pflag.FlagSet) {
	flags.StringVar(&opts.OCICredentialsFile, "oci-credentials-file", opts.OCICredentialsFile, "Path to the OCI credentials file (default: ~/.oci/config)")
	flags.StringVar(&opts.OCIKeyFile, "oci-key-file", opts.OCIKeyFile, "Path to the OCI API signing key file")
	flags.StringVar(&opts.Region, "oci-region", opts.Region, "OCI region (e.g., us-ashburn-1)")
	flags.StringVar(&opts.CompartmentID, "oci-compartment-id", opts.CompartmentID, "OCI compartment OCID")

	// NodePool options
	flags.StringVar(&opts.NodePoolOpts.InstanceShape, "oci-instance-shape", opts.NodePoolOpts.InstanceShape, "OCI instance shape (e.g., VM.Standard.E4.Flex)")
	flags.Int64Var(&opts.NodePoolOpts.OCPUs, "oci-ocpus", opts.NodePoolOpts.OCPUs, "Number of OCPUs for flexible shapes")
	flags.Int64Var(&opts.NodePoolOpts.MemoryInGBs, "oci-memory-in-gbs", opts.NodePoolOpts.MemoryInGBs, "Memory in GiB for flexible shapes")
	flags.StringVar(&opts.NodePoolOpts.AvailabilityDomain, "oci-availability-domain", opts.NodePoolOpts.AvailabilityDomain, "OCI availability domain (e.g., us-ashburn-1-AD-1)")
	flags.StringVar(&opts.NodePoolOpts.SubnetID, "oci-subnet-id", opts.NodePoolOpts.SubnetID, "OCI subnet OCID")
	flags.Int64Var(&opts.NodePoolOpts.BootVolumeSize, "oci-boot-volume-size", opts.NodePoolOpts.BootVolumeSize, "Boot volume size in GiB (default: 50)")
}

type RawCreateOptions struct {
	OCICredentialsFile string
	OCIKeyFile         string
	Region             string
	CompartmentID      string

	NodePoolOpts *OCINodePoolCreateOptions
}

type OCINodePoolCreateOptions struct {
	InstanceShape      string
	OCPUs              int64
	MemoryInGBs        int64
	AvailabilityDomain string
	SubnetID           string
	BootVolumeSize     int64
}

// validatedCreateOptions is a private wrapper that enforces a call of Validate() before Complete() can be invoked.
type validatedCreateOptions struct {
	*RawCreateOptions

	configData []byte
	keyData    []byte
}

type ValidatedCreateOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*validatedCreateOptions
}

// completedCreateOptions is a private wrapper that enforces a call of Complete() before cluster creation can be invoked.
type completedCreateOptions struct {
	*ValidatedCreateOptions

	name, namespace string
}

type CreateOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedCreateOptions
}

func (o *ValidatedCreateOptions) Complete(ctx context.Context, opts *core.CreateOptions) (core.Platform, error) {
	output := &CreateOptions{
		completedCreateOptions: &completedCreateOptions{
			ValidatedCreateOptions: o,
			name:                   opts.Name,
			namespace:              opts.Namespace,
		},
	}

	return output, nil
}

func (o *RawCreateOptions) Validate(ctx context.Context, opts *core.CreateOptions) (core.PlatformCompleter, error) {
	// Check that the OCI credentials file arg is set and that the file exists
	if o.OCICredentialsFile != "" {
		if _, err := os.Stat(o.OCICredentialsFile); err != nil {
			return nil, fmt.Errorf("oci credentials file does not exist: %w", err)
		}
	} else {
		credentialsFile, err := findOCICredentialsFile()
		if err != nil {
			return nil, fmt.Errorf("failed to find oci config file: %w", err)
		}
		if credentialsFile == "" {
			return nil, fmt.Errorf("oci credentials file not specified and ~/.oci/config not found")
		}
		o.OCICredentialsFile = credentialsFile
	}

	// Check that the OCI key file exists
	if o.OCIKeyFile == "" {
		return nil, fmt.Errorf("oci key file must be specified with --oci-key-file")
	}
	if _, err := os.Stat(o.OCIKeyFile); err != nil {
		return nil, fmt.Errorf("oci key file does not exist: %w", err)
	}

	// Validate region is not empty
	if o.Region == "" {
		return nil, fmt.Errorf("oci region must be specified with --oci-region")
	}

	// Validate compartment ID is not empty
	if o.CompartmentID == "" {
		return nil, fmt.Errorf("oci compartment ID must be specified with --oci-compartment-id")
	}

	if err := util.ValidateRequiredOption("pull-secret", opts.PullSecretFile); err != nil {
		return nil, err
	}

	// Read credential file contents
	configData, err := os.ReadFile(o.OCICredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read oci credentials file: %w", err)
	}

	// Read key file contents
	keyData, err := os.ReadFile(o.OCIKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read oci key file: %w", err)
	}

	validOpts := &ValidatedCreateOptions{
		validatedCreateOptions: &validatedCreateOptions{
			RawCreateOptions: o,
			configData:       configData,
			keyData:          keyData,
		},
	}

	return validOpts, nil
}

func (o *RawCreateOptions) ApplyPlatformSpecifics(cluster *hyperv1.HostedCluster) error {
	cluster.Spec.Platform = hyperv1.PlatformSpec{
		Type: hyperv1.OCIPlatform,
		OCI: &hyperv1.OCIPlatformSpec{
			IdentityRef: hyperv1.OCIIdentityReference{
				Name: credentialsSecret(cluster.Namespace, cluster.Name).Name,
			},
			Region:        o.Region,
			CompartmentID: o.CompartmentID,
		},
	}

	cluster.Spec.Services = core.GetIngressServicePublishingStrategyMapping(cluster.Spec.Networking.NetworkType, false)

	return nil
}

func (o *CreateOptions) GenerateNodePools(constructor core.DefaultNodePoolConstructor) []*hyperv1.NodePool {
	nodePool := constructor(hyperv1.OCIPlatform, "")
	if nodePool.Spec.Management.UpgradeType == "" {
		nodePool.Spec.Management.UpgradeType = hyperv1.UpgradeTypeReplace
	}

	ociPlatform := &hyperv1.OCINodePoolPlatform{
		InstanceShape:      o.NodePoolOpts.InstanceShape,
		AvailabilityDomain: o.NodePoolOpts.AvailabilityDomain,
		SubnetID:           o.NodePoolOpts.SubnetID,
	}

	// Add instance shape config for flexible shapes if OCPUs and memory are specified
	if o.NodePoolOpts.OCPUs > 0 && o.NodePoolOpts.MemoryInGBs > 0 {
		ociPlatform.InstanceShapeConfig = &hyperv1.OCIInstanceShapeConfig{
			OCPUs:       o.NodePoolOpts.OCPUs,
			MemoryInGBs: o.NodePoolOpts.MemoryInGBs,
		}
	}

	// Add boot volume size if specified
	if o.NodePoolOpts.BootVolumeSize > 0 {
		ociPlatform.BootVolumeSize = &o.NodePoolOpts.BootVolumeSize
	}

	nodePool.Spec.Platform.OCI = ociPlatform
	return []*hyperv1.NodePool{nodePool}
}

func (o *CreateOptions) GenerateResources() ([]client.Object, error) {
	resources := []client.Object{}

	credentialsSecret := credentialsSecret(o.namespace, o.name)
	credentialsSecret.Data = map[string][]byte{
		"config": o.configData,
		"key":    o.keyData,
	}

	resources = append(resources, credentialsSecret)

	return resources, nil
}

func credentialsSecret(namespace, name string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-oci-creds",
			Namespace: namespace,
			Labels:    map[string]string{util.DeleteWithClusterLabelName: "true"},
		},
		Type: corev1.SecretTypeOpaque,
	}
}

var _ core.Platform = (*CreateOptions)(nil)

func NewCreateCommand(opts *core.RawCreateOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "oci",
		Short:        "Creates basic functional HostedCluster resources on OCI",
		SilenceUsage: true,
	}

	ociOpts := DefaultOptions()
	BindOptions(ociOpts, cmd.Flags())
	_ = cmd.MarkPersistentFlagRequired("pull-secret")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if opts.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
			defer cancel()
		}

		if err := core.CreateCluster(ctx, opts, ociOpts); err != nil {
			opts.Log.Error(err, "Failed to create cluster")
			return err
		}
		return nil
	}

	return cmd
}

// findOCICredentialsFile searches for an OCI config file in the standard location,
// returning the first match found else the empty string
func findOCICredentialsFile() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(homeDir, ".oci", "config")

	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", nil
}
