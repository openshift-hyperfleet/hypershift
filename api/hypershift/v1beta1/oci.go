package v1beta1

// OCIPlatformSpec specifies configuration for clusters running on Oracle Cloud Infrastructure.
type OCIPlatformSpec struct {
	// identityRef is a reference to a secret holding OCI credentials
	// to be used when reconciling the hosted cluster.
	// The secret must contain two keys:
	//   - "config": OCI configuration file content (typically ~/.oci/config format)
	//   - "key": OCI API signing key (PEM-encoded private key)
	//
	// +required
	IdentityRef OCIIdentityReference `json:"identityRef"`

	// region is the OCI region in which the cluster resides.
	// A valid region must satisfy the following rules:
	//   format: Must be in the form `<countryCode>-<location>-<number>`
	//   characters: Only lowercase letters (a-z), digits (0-9), and hyphens (-) are allowed
	//   valid examples: "us-ashburn-1", "us-phoenix-1", "eu-frankfurt-1", "ap-tokyo-1"
	// For a full list of valid regions, see: https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm.
	//
	// +required
	// +immutable
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:Pattern=`^[a-z]+-[a-z]+-[0-9]+$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Region is immutable"
	Region string `json:"region"`

	// compartmentID is the OCI compartment OCID where the cluster resides.
	// A valid compartment OCID must satisfy the following rules:
	//   format: Must be in the form `ocid1.compartment.oc1..<unique_ID>`
	//   characters: Only lowercase letters (a-z), digits (0-9), and periods (.) are allowed
	//   start: Must begin with `ocid1.compartment.oc1..`
	//   valid examples: "ocid1.compartment.oc1..aaaaaaaazgovbe2qxduadk3bmj5dobvoe5wnengzavax5pwsfr3bqbdrrcqa".
	// For more information about compartment OCIDs, see: https://docs.oracle.com/en-us/iaas/Content/General/Concepts/identifiers.htm.
	//
	// +required
	// +immutable
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:Pattern=`^ocid1\.compartment\.oc1\.\.[a-z0-9]+$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="CompartmentID is immutable"
	CompartmentID string `json:"compartmentId"`
}

// OCINodePoolPlatform specifies the configuration of a NodePool when operating
// on the OCI platform.
type OCINodePoolPlatform struct {
	// instanceShape is the OCI instance shape to use for node instances.
	// An instance shape determines the number of CPUs, amount of memory,
	// and other resources allocated to the instance.
	// Valid examples: "VM.Standard.E4.Flex", "VM.Standard3.Flex", "VM.Standard.A1.Flex"
	// For a full list of shapes, see: https://docs.oracle.com/en-us/iaas/Content/Compute/References/computeshapes.htm.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	InstanceShape string `json:"instanceShape"`

	// instanceShapeConfig defines the configuration for a flexible instance shape.
	// This is required when using a flexible shape (e.g., VM.Standard.E4.Flex).
	//
	// +optional
	InstanceShapeConfig *OCIInstanceShapeConfig `json:"instanceShapeConfig,omitempty"`

	// availabilityDomain is the OCI availability domain in which to place node instances.
	// Each OCI region has one or more availability domains.
	// Format: "<region>-AD-<number>" (e.g., "us-ashburn-1-AD-1")
	// For more information, see: https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	AvailabilityDomain string `json:"availabilityDomain"`

	// subnetID is the OCID of the subnet in which to place node instances.
	// The subnet must be in the same VCN as the cluster's networking resources.
	// A valid subnet OCID must be in the form "ocid1.subnet.oc1.<region>.<unique_ID>"
	// where <region> is the OCI region identifier (e.g., "us-sanjose-1").
	// For more information, see: https://docs.oracle.com/en-us/iaas/Content/General/Concepts/identifiers.htm.
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:Pattern=`^ocid1\.subnet\.oc1\.[a-z0-9.-]+[a-z0-9]+$`
	SubnetID string `json:"subnetId"`

	// bootVolumeSize is the size of the boot volume in GiB for node instances.
	// If unspecified, the default boot volume size for the instance shape is used.
	//
	// +optional
	// +kubebuilder:validation:Minimum=50
	// +kubebuilder:validation:Maximum=32768
	BootVolumeSize *int64 `json:"bootVolumeSize,omitempty"`

	// imageID is the OCID of the custom image to use for node instances.
	// If unspecified, the default RHCOS image will be used based on the
	// NodePool release payload.
	// A valid image OCID must be in the form "ocid1.image.oc1.<region>.<unique_ID>"
	// where <region> is the OCI region identifier (e.g., "us-sanjose-1").
	// For more information, see: https://docs.oracle.com/en-us/iaas/Content/General/Concepts/identifiers.htm.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:Pattern=`^ocid1\.image\.oc1\.[a-z0-9.-]+[a-z0-9]+$`
	ImageID *string `json:"imageId,omitempty"`
}

// OCIInstanceShapeConfig defines the configuration for a flexible OCI instance shape.
type OCIInstanceShapeConfig struct {
	// ocpus is the number of OCPUs to allocate for the instance.
	// An OCPU is equivalent to one physical CPU core (two vCPUs for most shapes).
	//
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=114
	OCPUs int64 `json:"ocpus"`

	// memoryInGBs is the amount of memory in GiB to allocate for the instance.
	// The minimum and maximum values depend on the number of OCPUs.
	// Generally, the ratio is 1 OCPU to 1-64 GiB of memory.
	//
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1760
	MemoryInGBs int64 `json:"memoryInGBs"`
}

// OCIIdentityReference is a reference to a secret containing OCI credentials.
type OCIIdentityReference struct {
	// name is the name of a secret in the same namespace as the HostedCluster.
	// The secret must contain the following keys:
	//   - "config": OCI configuration file content
	//   - "key": OCI API signing key (PEM format)
	//
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}
