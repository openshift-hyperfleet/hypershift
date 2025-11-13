package oci

import (
	"context"
	"fmt"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/hypershift/support/upsert"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// OCI credentials secret keys
	credentialsConfigKey = "config"
	credentialsKeyKey    = "key"
)

type OCI struct {
	capiProviderImage string
}

func New(capiProviderImage string) *OCI {
	return &OCI{
		capiProviderImage: capiProviderImage,
	}
}

// ReconcileCAPIInfraCR reconciles the CAPI infrastructure cluster resource for OCI.
// For the MVP, this returns nil because we don't yet have CAPOCI provider types imported.
// This will be implemented in a future phase when we integrate the actual CAPI OCI provider.
func (o OCI) ReconcileCAPIInfraCR(ctx context.Context, c client.Client, createOrUpdate upsert.CreateOrUpdateFN,
	hcluster *hyperv1.HostedCluster, controlPlaneNamespace string, apiEndpoint hyperv1.APIEndpoint) (client.Object, error) {

	// TODO(Phase 2): Implement OCICluster CR creation
	// For now, return nil as we don't have CAPOCI types imported yet.
	// This allows the HostedCluster to reconcile without errors.
	return nil, nil
}

// CAPIProviderDeploymentSpec returns the deployment spec for the CAPI OCI provider.
// This configures the CAPOCI controller that will watch OCICluster and OCIMachine resources.
func (o OCI) CAPIProviderDeploymentSpec(hcluster *hyperv1.HostedCluster, _ *hyperv1.HostedControlPlane) (*appsv1.DeploymentSpec, error) {
	// Use the provider image from the constructor, with override support
	capiProviderImage := o.capiProviderImage
	if override, ok := hcluster.Annotations["hypershift.openshift.io/capi-provider-oci-image"]; ok {
		capiProviderImage = override
	}

	// If no image is specified, we can't deploy the provider
	if capiProviderImage == "" {
		return nil, fmt.Errorf("CAPI OCI provider image not specified")
	}

	deploymentSpec := &appsv1.DeploymentSpec{
		Replicas: ptr.To[int32](1),
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				TerminationGracePeriodSeconds: ptr.To[int64](10),
				Volumes: []corev1.Volume{
					{
						Name: "credentials",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "oci-credentials",
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "manager",
						Image:           capiProviderImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/manager"},
						Args: []string{
							"--namespace=$(MY_NAMESPACE)",
							"--leader-elect",
							"--v=2",
						},
						Env: []corev1.EnvVar{
							{
								Name: "MY_NAMESPACE",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.namespace",
									},
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "credentials",
								MountPath: "/credentials",
								ReadOnly:  true,
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "healthz",
								ContainerPort: 9440,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromString("healthz"),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readyz",
									Port: intstr.FromString("healthz"),
								},
							},
						},
					},
				},
			},
		},
	}

	return deploymentSpec, nil
}

// ReconcileCredentials syncs OCI credentials from the HostedCluster namespace
// to the control plane namespace for use by the CAPI provider.
func (o OCI) ReconcileCredentials(ctx context.Context, c client.Client, createOrUpdate upsert.CreateOrUpdateFN,
	hcluster *hyperv1.HostedCluster, controlPlaneNamespace string) error {

	// Validate platform spec
	if hcluster.Spec.Platform.OCI == nil {
		return fmt.Errorf("OCI platform spec is nil")
	}

	// Source secret in HostedCluster namespace
	sourceSecret := &corev1.Secret{}
	sourceSecretName := client.ObjectKey{
		Namespace: hcluster.Namespace,
		Name:      hcluster.Spec.Platform.OCI.IdentityRef.Name,
	}

	if err := c.Get(ctx, sourceSecretName, sourceSecret); err != nil {
		return fmt.Errorf("failed to get OCI credentials secret %q: %w", sourceSecretName.Name, err)
	}

	// Validate secret has required keys
	if _, hasConfig := sourceSecret.Data[credentialsConfigKey]; !hasConfig {
		return fmt.Errorf("OCI credentials secret missing '%s' key", credentialsConfigKey)
	}
	if _, hasKey := sourceSecret.Data[credentialsKeyKey]; !hasKey {
		return fmt.Errorf("OCI credentials secret missing '%s' key", credentialsKeyKey)
	}

	// Target secret in control plane namespace
	// Use the same name as the source secret for consistency
	targetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hcluster.Spec.Platform.OCI.IdentityRef.Name,
			Namespace: controlPlaneNamespace,
		},
	}

	_, err := createOrUpdate(ctx, c, targetSecret, func() error {
		targetSecret.Type = corev1.SecretTypeOpaque
		targetSecret.Data = map[string][]byte{
			credentialsConfigKey: sourceSecret.Data[credentialsConfigKey],
			credentialsKeyKey:    sourceSecret.Data[credentialsKeyKey],
		}
		return nil
	})

	return err
}

// ReconcileSecretEncryption configures secret encryption for the hosted cluster.
// For MVP, this is a no-op. OCI Vault integration can be added in future releases.
func (o OCI) ReconcileSecretEncryption(ctx context.Context, c client.Client, createOrUpdate upsert.CreateOrUpdateFN,
	hcluster *hyperv1.HostedCluster, controlPlaneNamespace string) error {
	// OCI Vault integration is a future enhancement
	return nil
}

// CAPIProviderPolicyRules returns the RBAC policy rules required by the CAPI OCI provider.
func (o OCI) CAPIProviderPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{"infrastructure.cluster.x-k8s.io"},
			Resources: []string{
				"ociclusters",
				"ociclusters/status",
				"ocimachines",
				"ocimachines/status",
				"ocimachinetemplates",
			},
			Verbs: []string{rbacv1.VerbAll},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}
}

// DeleteCredentials removes OCI credentials from the control plane namespace
// when the HostedCluster is being deleted.
func (o OCI) DeleteCredentials(ctx context.Context, c client.Client, hcluster *hyperv1.HostedCluster, controlPlaneNamespace string) error {
	// Skip if platform spec is nil (shouldn't happen in normal deletion flow)
	if hcluster.Spec.Platform.OCI == nil {
		return nil
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hcluster.Spec.Platform.OCI.IdentityRef.Name,
			Namespace: controlPlaneNamespace,
		},
	}

	if err := c.Delete(ctx, credentialsSecret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete OCI credentials: %w", err)
	}

	return nil
}
