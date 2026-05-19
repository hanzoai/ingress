// ingress.zap — ZAP schema for the Hanzo Ingress subsystem.
//
// HIP-0106 unified-binary contract. Hanzo Ingress is k8s-native: the
// full reverse-proxy / TLS / provider machinery runs as the standalone
// hanzo-ingress binary at the cluster edge. The unified cloud binary
// only co-resides for healthz coverage and a read-only config view —
// the Mount surface here matches that scope.

package ingress.v1;

service Health {
  // Check returns OK when the ingress mount is wired.
  Check(HealthRequest) -> HealthResponse;
}

message HealthRequest {}

message HealthResponse {
  string status = 1;  // "ok"
  string service = 2; // "ingress"
}

service Config {
  // Get returns a read-only projection of the ingress runtime config
  // for this deployment. Mirrors /_/ingress/config.
  Get(ConfigRequest) -> ConfigView;
}

message ConfigRequest {}

message ConfigView {
  string brand = 1;
  string domain = 2;
  repeated string entrypoints = 3;
  repeated string providers = 4;
  bool acme_enabled = 5;
}
