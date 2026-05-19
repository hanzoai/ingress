// Copyright © 2026 Hanzo AI. MIT License.

// Package ingress is the HIP-0106 unified-binary mount surface for
// the Hanzo Ingress subsystem.
//
// The mount logic ships under the `cloud` build tag so the standalone
// cmd/traefik binary (the full hanzo-ingress reverse proxy that runs
// at the k8s cluster edge) builds without pulling hanzoai/cloud +
// hanzoai/zip into its dependency tree. Build with `-tags=cloud` to
// compile the Mount.
//
// See pkg/ingress/mount.go for the canonical
//
//	func Mount(app *zip.App, deps cloud.Deps) error
//
// signature and the init() callsite that registers ("ingress", order=90)
// in cloud.Registry.
package ingress
