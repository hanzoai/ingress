package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kschema "k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the group name for the hanzoai Traefik-fork CRDs
// (Middleware, IngressRoute, TLSOption, ServersTransport, etc.).
//
// Was "ingress.k8s.io" — but every cluster installs the CRDs under
// `hanzo.ai` (see baseapps.hanzo.ai, ingressroutes.hanzo.ai,
// middlewares.hanzo.ai, etc.). With the previous string, the
// controller's informer watched a group that doesn't exist on cluster:
//
//	"failed to list *v1alpha1.Middleware: middlewares.ingress.k8s.io is
//	 forbidden: User 'system:serviceaccount:liquidity:ingress' cannot
//	 list resource 'middlewares' in API group 'ingress.k8s.io'"
//
// Result: every rewrite/headers/redirect Middleware referenced by a
// standard K8s Ingress via the
// `traefik.ingress.kubernetes.io/router.middlewares` annotation was
// silently ignored — the route still landed at the backend but no
// rewrite ever ran. Latent for any deployment using middleware-based
// path rewrites.
//
// Aligning to "hanzo.ai" matches the cluster CRDs + kubectl resource
// discovery + the operator's naming convention.
const GroupName = "hanzo.ai"

var (
	// SchemeBuilder collects the scheme builder functions.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme applies the SchemeBuilder functions to a specified scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// SchemeGroupVersion is group version used to register these objects.
var SchemeGroupVersion = kschema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind.
func Kind(kind string) kschema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource.
func Resource(resource string) kschema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&IngressRoute{},
		&IngressRouteList{},
		&IngressRouteTCP{},
		&IngressRouteTCPList{},
		&IngressRouteUDP{},
		&IngressRouteUDPList{},
		&Middleware{},
		&MiddlewareList{},
		&MiddlewareTCP{},
		&MiddlewareTCPList{},
		&TLSOption{},
		&TLSOptionList{},
		&TLSStore{},
		&TLSStoreList{},
		&IngressService{},
		&IngressServiceList{},
		&ServersTransport{},
		&ServersTransportList{},
		&ServersTransportTCP{},
		&ServersTransportTCPList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
