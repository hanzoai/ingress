package cmd

import (
	"time"

	ptypes "github.com/traefik/paerser/types"
	"github.com/hanzoai/ingress/v3/pkg/config/static"
)

// IngressCmdConfiguration wraps the static configuration and extra parameters.
type IngressCmdConfiguration struct {
	static.Configuration `export:"true"`

	// ConfigFile is the path to the configuration file.
	ConfigFile string `description:"Configuration file to use. If specified all other flags are ignored." export:"true"`
}

// NewIngressConfiguration creates a IngressCmdConfiguration with default values.
func NewIngressConfiguration() *IngressCmdConfiguration {
	return &IngressCmdConfiguration{
		Configuration: static.Configuration{
			Global: &static.Global{
				CheckNewVersion: true,
			},
			EntryPoints: make(static.EntryPoints),
			Providers: &static.Providers{
				ProvidersThrottleDuration: ptypes.Duration(2 * time.Second),
			},
			ServersTransport: &static.ServersTransport{
				MaxIdleConnsPerHost: 200,
			},
			TCPServersTransport: &static.TCPServersTransport{
				DialTimeout:   ptypes.Duration(30 * time.Second),
				DialKeepAlive: ptypes.Duration(15 * time.Second),
			},
		},
		ConfigFile: "",
	}
}
