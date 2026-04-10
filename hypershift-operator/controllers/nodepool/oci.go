package nodepool

import (
	"fmt"
	"strconv"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capioci "github.com/oracle/cluster-api-provider-oci/api/v1beta2"
)

func (c *CAPI) ociMachineTemplate(templateNameGenerator func(spec any) (string, error)) (*capioci.OCIMachineTemplate, error) {
	nodePool := c.nodePool

	spec, err := ociMachineTemplateSpec(c.hostedCluster, nodePool)
	if err != nil {
		return nil, err
	}

	templateName, err := templateNameGenerator(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to generate template name: %w", err)
	}

	template := &capioci.OCIMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: templateName,
		},
		Spec: *spec,
	}

	return template, nil
}

func ociMachineTemplateSpec(hcluster *hyperv1.HostedCluster, nodePool *hyperv1.NodePool) (*capioci.OCIMachineTemplateSpec, error) {
	if nodePool.Spec.Platform.OCI == nil {
		return nil, fmt.Errorf("OCI platform spec is required for OCI NodePools")
	}
	ociPlatform := nodePool.Spec.Platform.OCI

	machineSpec := capioci.OCIMachineSpec{
		Shape:         ociPlatform.InstanceShape,
		CompartmentId: hcluster.Spec.Platform.OCI.CompartmentID,
	}

	// Map flexible shape configuration
	if ociPlatform.InstanceShapeConfig != nil {
		machineSpec.ShapeConfig = capioci.ShapeConfig{
			Ocpus:       strconv.FormatInt(ociPlatform.InstanceShapeConfig.OCPUs, 10),
			MemoryInGBs: strconv.FormatInt(ociPlatform.InstanceShapeConfig.MemoryInGBs, 10),
		}
	}

	// Map boot volume size
	if ociPlatform.BootVolumeSize != nil {
		machineSpec.BootVolumeSizeInGBs = strconv.FormatInt(*ociPlatform.BootVolumeSize, 10)
	}

	// Map custom image
	if ociPlatform.ImageID != nil {
		machineSpec.ImageId = *ociPlatform.ImageID
	}

	// Map subnet by using the SubnetID as the subnet name reference
	// CAPOCI resolves subnets by name from the OCICluster spec
	machineSpec.SubnetName = ociPlatform.SubnetID

	return &capioci.OCIMachineTemplateSpec{
		Template: capioci.OCIMachineTemplateResource{
			Spec: machineSpec,
		},
	}, nil
}
