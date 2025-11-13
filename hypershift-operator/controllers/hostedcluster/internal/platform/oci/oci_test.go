package oci

import (
	"context"
	"testing"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcileCAPIInfraCR(t *testing.T) {
	ctx := context.Background()
	hcluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-oci",
			Namespace: "clusters",
		},
		Spec: hyperv1.HostedClusterSpec{
			Platform: hyperv1.PlatformSpec{
				Type: hyperv1.OCIPlatform,
				OCI: &hyperv1.OCIPlatformSpec{
					IdentityRef: hyperv1.OCIIdentityReference{
						Name: "oci-credentials",
					},
					CompartmentID: "ocid1.compartment.oc1..aaaaaa123",
					Region:        "us-sanjose-1",
				},
			},
		},
	}

	apiEndpoint := hyperv1.APIEndpoint{
		Host: "api.test.hypershift.local",
		Port: 6443,
	}

	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	oci := New("")
	createOrUpdate := func(ctx context.Context, c client.Client, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
		return controllerutil.OperationResultNone, f()
	}

	// For MVP, this should return nil without error
	result, err := oci.ReconcileCAPIInfraCR(ctx, fakeClient, createOrUpdate, hcluster, "clusters-test-oci", apiEndpoint)

	if err != nil {
		t.Errorf("ReconcileCAPIInfraCR() error = %v, want nil", err)
	}
	if result != nil {
		t.Errorf("ReconcileCAPIInfraCR() result = %v, want nil (MVP implementation)", result)
	}
}

func TestCAPIProviderDeploymentSpec(t *testing.T) {
	tests := []struct {
		name          string
		providerImage string
		annotation    map[string]string
		wantErr       bool
		wantImage     string
	}{
		{
			name:          "with provider image",
			providerImage: "test-image:v1.0.0",
			wantErr:       false,
			wantImage:     "test-image:v1.0.0",
		},
		{
			name:          "without provider image",
			providerImage: "",
			wantErr:       true,
		},
		{
			name:          "with annotation override",
			providerImage: "test-image:v1.0.0",
			annotation: map[string]string{
				"hypershift.openshift.io/capi-provider-oci-image": "override-image:v2.0.0",
			},
			wantErr:   false,
			wantImage: "override-image:v2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hcluster := &hyperv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotation,
				},
			}
			hcp := &hyperv1.HostedControlPlane{}

			oci := New(tt.providerImage)
			spec, err := oci.CAPIProviderDeploymentSpec(hcluster, hcp)

			if (err != nil) != tt.wantErr {
				t.Errorf("CAPIProviderDeploymentSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if spec == nil {
					t.Errorf("CAPIProviderDeploymentSpec() spec = nil, want non-nil")
					return
				}
				if len(spec.Template.Spec.Containers) == 0 {
					t.Errorf("CAPIProviderDeploymentSpec() no containers found")
					return
				}
				if spec.Template.Spec.Containers[0].Image != tt.wantImage {
					t.Errorf("CAPIProviderDeploymentSpec() image = %v, want %v",
						spec.Template.Spec.Containers[0].Image, tt.wantImage)
				}
			}
		})
	}
}

func TestReconcileCredentials(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		sourceSecret  *corev1.Secret
		wantErr       bool
		errorContains string
	}{
		{
			name: "valid credentials",
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-credentials",
					Namespace: "clusters",
				},
				Data: map[string][]byte{
					"config": []byte("test-config"),
					"key":    []byte("test-key"),
				},
			},
			wantErr: false,
		},
		{
			name: "missing config key",
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-credentials",
					Namespace: "clusters",
				},
				Data: map[string][]byte{
					"key": []byte("test-key"),
				},
			},
			wantErr:       true,
			errorContains: "missing 'config' key",
		},
		{
			name: "missing key key",
			sourceSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-credentials",
					Namespace: "clusters",
				},
				Data: map[string][]byte{
					"config": []byte("test-config"),
				},
			},
			wantErr:       true,
			errorContains: "missing 'key' key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.sourceSecret != nil {
				builder = builder.WithObjects(tt.sourceSecret)
			}
			fakeClient := builder.Build()

			hcluster := &hyperv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-oci",
					Namespace: "clusters",
				},
				Spec: hyperv1.HostedClusterSpec{
					Platform: hyperv1.PlatformSpec{
						Type: hyperv1.OCIPlatform,
						OCI: &hyperv1.OCIPlatformSpec{
							IdentityRef: hyperv1.OCIIdentityReference{
								Name: "oci-credentials",
							},
						},
					},
				},
			}

			targetSecretCreated := false
			createOrUpdate := func(ctx context.Context, c client.Client, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				targetSecretCreated = true
				return controllerutil.OperationResultCreated, f()
			}

			oci := New("")
			err := oci.ReconcileCredentials(ctx, fakeClient, createOrUpdate, hcluster, "clusters-test-oci")

			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errorContains != "" {
				if err == nil || err.Error() == "" {
					t.Errorf("ReconcileCredentials() expected error containing %q, got nil", tt.errorContains)
				} else if !contains(err.Error(), tt.errorContains) {
					t.Errorf("ReconcileCredentials() error = %v, want error containing %q", err, tt.errorContains)
				}
			}

			if !tt.wantErr && !targetSecretCreated {
				t.Errorf("ReconcileCredentials() did not create target secret")
			}
		})
	}
}

func TestReconcileSecretEncryption(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	hcluster := &hyperv1.HostedCluster{}
	createOrUpdate := func(ctx context.Context, c client.Client, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
		return controllerutil.OperationResultNone, nil
	}

	oci := New("")
	err := oci.ReconcileSecretEncryption(ctx, fakeClient, createOrUpdate, hcluster, "test-namespace")

	// Should be a no-op for MVP
	if err != nil {
		t.Errorf("ReconcileSecretEncryption() error = %v, want nil", err)
	}
}

func TestCAPIProviderPolicyRules(t *testing.T) {
	oci := New("")
	rules := oci.CAPIProviderPolicyRules()

	if len(rules) == 0 {
		t.Errorf("CAPIProviderPolicyRules() returned empty rules")
	}

	// Check that infrastructure.cluster.x-k8s.io resources are present
	foundInfra := false
	for _, rule := range rules {
		for _, apiGroup := range rule.APIGroups {
			if apiGroup == "infrastructure.cluster.x-k8s.io" {
				foundInfra = true
				break
			}
		}
	}

	if !foundInfra {
		t.Errorf("CAPIProviderPolicyRules() missing infrastructure.cluster.x-k8s.io API group")
	}
}

func TestDeleteCredentials(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		existingSecret bool
		wantErr        bool
	}{
		{
			name:           "delete existing secret",
			existingSecret: true,
			wantErr:        false,
		},
		{
			name:           "delete non-existing secret (no error)",
			existingSecret: false,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.existingSecret {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "oci-credentials",
						Namespace: "test-namespace",
					},
				}
				builder = builder.WithObjects(secret)
			}
			fakeClient := builder.Build()

			hcluster := &hyperv1.HostedCluster{
				Spec: hyperv1.HostedClusterSpec{
					Platform: hyperv1.PlatformSpec{
						Type: hyperv1.OCIPlatform,
						OCI: &hyperv1.OCIPlatformSpec{
							IdentityRef: hyperv1.OCIIdentityReference{
								Name: "oci-credentials",
							},
						},
					},
				},
			}
			oci := New("")

			err := oci.DeleteCredentials(ctx, fakeClient, hcluster, "test-namespace")

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify secret is deleted
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: "oci-credentials", Namespace: "test-namespace"}, secret)
			if !apierrors.IsNotFound(err) {
				t.Errorf("DeleteCredentials() secret still exists after deletion")
			}
		})
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || s[len(s)-len(substr):] == substr || s[:len(substr)] == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
