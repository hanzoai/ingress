package v1alpha1

// ObjectReference is a generic reference to a Ingress resource.
type ObjectReference struct {
	// Name defines the name of the referenced Ingress resource.
	Name string `json:"name"`
	// Namespace defines the namespace of the referenced Ingress resource.
	Namespace string `json:"namespace,omitempty"`
}
