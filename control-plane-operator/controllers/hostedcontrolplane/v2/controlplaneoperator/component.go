package controlplaneoperator

import (
	"time"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	component "github.com/openshift/hypershift/support/controlplane-component"

	configv1 "github.com/openshift/api/config/v1"
)

const (
	ComponentName = "control-plane-operator"
)

var _ component.ComponentOptions = &ControlPlaneOperatorOptions{}

type ControlPlaneOperatorOptions struct {
	HostedCluster *hyperv1.HostedCluster

	Image          string
	UtilitiesImage string
	HasUtilities   bool

	CertRotationScale           time.Duration
	RegistryOverrideCommandLine string
	OpenShiftRegistryOverrides  string
	DefaultIngressDomain        string

	FeatureSet                 configv1.FeatureSet
	EnableOCPClusterMonitoring bool
}

// IsRequestServing implements controlplanecomponent.ComponentOptions.
func (c *ControlPlaneOperatorOptions) IsRequestServing() bool {
	return false
}

// MultiZoneSpread implements controlplanecomponent.ComponentOptions.
func (c *ControlPlaneOperatorOptions) MultiZoneSpread() bool {
	return false
}

// NeedsManagementKASAccess implements controlplanecomponent.ComponentOptions.
func (c *ControlPlaneOperatorOptions) NeedsManagementKASAccess() bool {
	return true
}

func NewComponent(options *ControlPlaneOperatorOptions) component.ControlPlaneComponent {
	comp := component.NewDeploymentComponent(ComponentName, options).
		WithAdaptFunction(options.adaptDeployment).
		WithManifestAdapter(
			"role.yaml",
			component.WithAdaptFunction(adaptRole),
		)

	// Only add PodMonitor if platform monitoring is enabled
	if options.EnableOCPClusterMonitoring {
		comp = comp.WithManifestAdapter(
			"podmonitor.yaml",
			component.WithAdaptFunction(options.adaptPodMonitor),
		)
	}

	return comp.
		WithManifestAdapter(
			"rolebinding.yaml",
			component.SetHostedClusterAnnotation(),
		).
		WithManifestAdapter(
			"serviceaccount.yaml",
			component.SetHostedClusterAnnotation(),
		).
		InjectTokenMinterContainer(component.TokenMinterContainerOptions{
			TokenType:               component.CloudToken,
			ServiceAccountName:      "control-plane-operator",
			ServiceAccountNameSpace: "kube-system",
			KubeconfigSecretName:    "service-network-admin-kubeconfig",
		}).
		Build()
}
