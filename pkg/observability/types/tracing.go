package types

import (
	luxtrace "github.com/luxfi/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"strings"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog/log"
	ttypes "github.com/hanzoai/ingress/pkg/types"
	"github.com/hanzoai/ingress/pkg/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type TracingVerbosity string

const (
	MinimalVerbosity  TracingVerbosity = "minimal"
	DetailedVerbosity TracingVerbosity = "detailed"
)

func (v TracingVerbosity) Allows(verbosity TracingVerbosity) bool {
	switch v {
	case DetailedVerbosity:
		return verbosity == DetailedVerbosity || verbosity == MinimalVerbosity
	default:
		return verbosity == MinimalVerbosity
	}
}

// OTelTracing provides configuration settings for the open-telemetry tracer.
type OTelTracing struct {
	GRPC *OTelGRPC `description:"gRPC configuration for the OpenTelemetry collector." json:"grpc,omitempty" toml:"grpc,omitempty" yaml:"grpc,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true"`
	HTTP *OTelHTTP `description:"HTTP configuration for the OpenTelemetry collector." json:"http,omitempty" toml:"http,omitempty" yaml:"http,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true"`
}

// SetDefaults sets the default values.
func (c *OTelTracing) SetDefaults() {
	c.HTTP = &OTelHTTP{}
	c.HTTP.SetDefaults()
}

// Setup sets up the tracer.
func (c *OTelTracing) Setup(ctx context.Context, serviceName string, sampleRate float64, resourceAttributes map[string]string) (trace.Tracer, io.Closer, error) {
	// One transport: ZAP to o11y/pkg/zapreceiver. The GRPC/HTTP split existed to
	// pick an OTLP flavour, and both flavours pull gRPC and protobuf — they share
	// otlpconfig — so choosing between them never avoided either.
	exporter, err := c.setupExporter(serviceName)
	if err != nil {
		return nil, nil, fmt.Errorf("setting up exporter: %w", err)
	}

	var resAttrs []attribute.KeyValue
	for k, v := range resourceAttributes {
		resAttrs = append(resAttrs, attribute.String(k, v))
	}

	res, err := resource.New(ctx,
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithDetectors(ttypes.K8sAttributesDetector{}),
		// The following order allows the user to override the service name and version,
		// as well as any other attributes set by the above detectors.
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version.Version),
		),
		resource.WithAttributes(resAttrs...),
		// Use the environment variables to allow overriding above resource attributes.
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("building resource: %w", err)
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tracerProvider)

	log.Debug().Msg("OpenTelemetry tracer configured")

	return tracerProvider.Tracer("github.com/hanzoai/ingress"), &tpCloser{provider: tracerProvider}, err
}

// setupExporter builds the ZAP span exporter. Endpoint is host:port — the
// configured value may carry an http(s) scheme from the OTLP era, which has no
// meaning on this wire and is trimmed.
func (c *OTelTracing) setupExporter(serviceName string) (sdktrace.SpanExporter, error) {
	var endpoint string
	insecure := true
	switch {
	case c.GRPC != nil && c.GRPC.Endpoint != "":
		endpoint = c.GRPC.Endpoint
		insecure = c.GRPC.Insecure
	case c.HTTP != nil:
		endpoint = c.HTTP.Endpoint
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	if i := strings.IndexByte(endpoint, '/'); i >= 0 {
		endpoint = endpoint[:i]
	}
	return luxtrace.NewZAPExporter(
		luxtrace.ExporterConfig{Type: luxtrace.ZAP, Endpoint: endpoint, Insecure: insecure},
		serviceName, "",
	)
}


// tpCloser converts a TraceProvider into an io.Closer.
type tpCloser struct {
	provider *sdktrace.TracerProvider
}

func (t *tpCloser) Close() error {
	if t == nil {
		return nil
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()

	return t.provider.Shutdown(ctx)
}
