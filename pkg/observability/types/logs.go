package types

import (
	"errors"
	"context"
	"github.com/hanzoai/ingress-parser/types"
	otelsdk "go.opentelemetry.io/otel/sdk/log"
)

const (
	// AccessLogKeep is the keep string value.
	AccessLogKeep = "keep"
	// AccessLogDrop is the drop string value.
	AccessLogDrop = "drop"
	// AccessLogRedact is the redact string value.
	AccessLogRedact = "redact"
)

const (
	// CommonFormat is the common logging format (CLF).
	CommonFormat string = "common"
)

const OTelIngressServiceName = "ingress"

// IngressLog holds the configuration settings for the ingress logger.
type IngressLog struct {
	Level   string `description:"Log level set to ingress logs." json:"level,omitempty" toml:"level,omitempty" yaml:"level,omitempty" export:"true"`
	Format  string `description:"Ingress log format: json | common" json:"format,omitempty" toml:"format,omitempty" yaml:"format,omitempty" export:"true"`
	NoColor bool   `description:"When using the 'common' format, disables the colorized output." json:"noColor,omitempty" toml:"noColor,omitempty" yaml:"noColor,omitempty" export:"true"`

	FilePath   string `description:"Ingress log file path. Stdout is used when omitted or empty." json:"filePath,omitempty" toml:"filePath,omitempty" yaml:"filePath,omitempty"`
	MaxSize    int    `description:"Maximum size in megabytes of the log file before it gets rotated." json:"maxSize,omitempty" toml:"maxSize,omitempty" yaml:"maxSize,omitempty" export:"true"`
	MaxAge     int    `description:"Maximum number of days to retain old log files based on the timestamp encoded in their filename." json:"maxAge,omitempty" toml:"maxAge,omitempty" yaml:"maxAge,omitempty" export:"true"`
	MaxBackups int    `description:"Maximum number of old log files to retain." json:"maxBackups,omitempty" toml:"maxBackups,omitempty" yaml:"maxBackups,omitempty" export:"true"`
	Compress   bool   `description:"Determines if the rotated log files should be compressed using gzip." json:"compress,omitempty" toml:"compress,omitempty" yaml:"compress,omitempty" export:"true"`

	OTLP *OTelLog `description:"Settings for OpenTelemetry." json:"otlp,omitempty" toml:"otlp,omitempty" yaml:"otlp,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true"`
}

// SetDefaults sets the default values.
func (l *IngressLog) SetDefaults() {
	l.Format = CommonFormat
	l.Level = "ERROR"
}

// AccessLog holds the configuration settings for the access logger (middlewares/accesslog).
type AccessLog struct {
	FilePath      string            `description:"Access log file path. Stdout is used when omitted or empty." json:"filePath,omitempty" toml:"filePath,omitempty" yaml:"filePath,omitempty"`
	Format        string            `description:"Access log format: json, common, or genericCLF" json:"format,omitempty" toml:"format,omitempty" yaml:"format,omitempty" export:"true"`
	Filters       *AccessLogFilters `description:"Access log filters, used to keep only specific access logs." json:"filters,omitempty" toml:"filters,omitempty" yaml:"filters,omitempty" export:"true"`
	Fields        *AccessLogFields  `description:"AccessLogFields." json:"fields,omitempty" toml:"fields,omitempty" yaml:"fields,omitempty" export:"true"`
	BufferingSize int64             `description:"Number of access log lines to process in a buffered way." json:"bufferingSize,omitempty" toml:"bufferingSize,omitempty" yaml:"bufferingSize,omitempty" export:"true"`
	AddInternals  bool              `description:"Enables access log for internal services (ping, dashboard, etc...)." json:"addInternals,omitempty" toml:"addInternals,omitempty" yaml:"addInternals,omitempty" export:"true"`
	DualOutput    bool              `description:"Enables access log output alongside OTLP. By default, this output is disabled when OTLP is configured." json:"dualOutput,omitempty" toml:"dualOutput,omitempty" yaml:"dualOutput,omitempty" export:"true"`

	OTLP *OTelLog `description:"Settings for OpenTelemetry." json:"otlp,omitempty" toml:"otlp,omitempty" yaml:"otlp,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true"`
}

// SetDefaults sets the default values.
func (l *AccessLog) SetDefaults() {
	l.Format = CommonFormat
	l.FilePath = ""
	l.Filters = &AccessLogFilters{}
	l.Fields = &AccessLogFields{}
	l.Fields.SetDefaults()
}

// AccessLogFilters holds filters configuration.
type AccessLogFilters struct {
	StatusCodes   []string       `description:"Keep access logs with status codes in the specified range." json:"statusCodes,omitempty" toml:"statusCodes,omitempty" yaml:"statusCodes,omitempty" export:"true"`
	RetryAttempts bool           `description:"Keep access logs when at least one retry happened." json:"retryAttempts,omitempty" toml:"retryAttempts,omitempty" yaml:"retryAttempts,omitempty" export:"true"`
	MinDuration   types.Duration `description:"Keep access logs when request took longer than the specified duration." json:"minDuration,omitempty" toml:"minDuration,omitempty" yaml:"minDuration,omitempty" export:"true"`
}

// FieldHeaders holds configuration for access log headers.
type FieldHeaders struct {
	DefaultMode string            `description:"Default mode for fields: keep | drop | redact" json:"defaultMode,omitempty" toml:"defaultMode,omitempty" yaml:"defaultMode,omitempty" export:"true"`
	Names       map[string]string `description:"Override mode for headers" json:"names,omitempty" toml:"names,omitempty" yaml:"names,omitempty" export:"true"`
}

// AccessLogFields holds configuration for access log fields.
type AccessLogFields struct {
	DefaultMode string            `description:"Default mode for fields: keep | drop" json:"defaultMode,omitempty" toml:"defaultMode,omitempty" yaml:"defaultMode,omitempty"  export:"true"`
	Names       map[string]string `description:"Override mode for fields" json:"names,omitempty" toml:"names,omitempty" yaml:"names,omitempty" export:"true"`
	Headers     *FieldHeaders     `description:"Headers to keep, drop or redact" json:"headers,omitempty" toml:"headers,omitempty" yaml:"headers,omitempty" export:"true"`
}

// SetDefaults sets the default values.
func (f *AccessLogFields) SetDefaults() {
	f.DefaultMode = AccessLogKeep
	f.Headers = &FieldHeaders{
		DefaultMode: AccessLogDrop,
	}
}

// Keep check if the field need to be kept or dropped.
func (f *AccessLogFields) Keep(field string) bool {
	defaultKeep := true
	if f != nil {
		defaultKeep = checkFieldValue(f.DefaultMode, defaultKeep)

		if v, ok := f.Names[field]; ok {
			return checkFieldValue(v, defaultKeep)
		}
	}
	return defaultKeep
}

// KeepHeader checks if the headers need to be kept, dropped or redacted and returns the status.
func (f *AccessLogFields) KeepHeader(header string) string {
	defaultValue := AccessLogKeep
	if f != nil && f.Headers != nil {
		defaultValue = checkFieldHeaderValue(f.Headers.DefaultMode, defaultValue)

		if v, ok := f.Headers.Names[header]; ok {
			return checkFieldHeaderValue(v, defaultValue)
		}
	}
	return defaultValue
}

func checkFieldValue(value string, defaultKeep bool) bool {
	switch value {
	case AccessLogKeep:
		return true
	case AccessLogDrop:
		return false
	default:
		return defaultKeep
	}
}

func checkFieldHeaderValue(value, defaultValue string) string {
	if value == AccessLogKeep || value == AccessLogDrop || value == AccessLogRedact {
		return value
	}
	return defaultValue
}

// OTelLog provides configuration settings for the open-telemetry logger.
type OTelLog struct {
	ServiceName        string            `description:"Defines the service name resource attribute." json:"serviceName,omitempty" toml:"serviceName,omitempty" yaml:"serviceName,omitempty" export:"true"`
	ResourceAttributes map[string]string `description:"Defines additional resource attributes (key:value)." json:"resourceAttributes,omitempty" toml:"resourceAttributes,omitempty" yaml:"resourceAttributes,omitempty"`
	GRPC               *OTelGRPC         `description:"gRPC configuration for the OpenTelemetry collector." json:"grpc,omitempty" toml:"grpc,omitempty" yaml:"grpc,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true"`
	HTTP               *OTelHTTP         `description:"HTTP configuration for the OpenTelemetry collector." json:"http,omitempty" toml:"http,omitempty" yaml:"http,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true"`
}

// SetDefaults sets the default values.
func (o *OTelLog) SetDefaults() {
	o.ServiceName = OTelIngressServiceName
	o.HTTP = &OTelHTTP{}
	o.HTTP.SetDefaults()
}

// NewLoggerProvider creates a new OpenTelemetry logger provider.
func (o *OTelLog) NewLoggerProvider(ctx context.Context) (*otelsdk.LoggerProvider, error) {
	// Logs have no ZAP wire. Traces ride MsgSpanBatch and metrics ride the
	// MetricFamily batch, but nothing on the o11y side receives a log batch, so
	// the only way to push logs from here was OTLP — and OTLP carries protobuf
	// and gRPC on either flavour. ingress writes to stdout; the platform
	// collects it. Refuse rather than pretend to ship.
	return nil, errors.New("otel log export is not available: no ZAP log wire exists — ingress logs to stdout")
}


