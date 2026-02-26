package healthcheck

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/traefik/paerser/cli"
	"github.com/hanzoai/ingress/v3/pkg/config/static"
)

// NewCmd builds a new HealthCheck command.
func NewCmd(ingressConfiguration *static.Configuration, loaders []cli.ResourceLoader) *cli.Command {
	return &cli.Command{
		Name:          "healthcheck",
		Description:   `Calls Traefik /ping endpoint (disabled by default) to check the health of Traefik.`,
		Configuration: ingressConfiguration,
		Run:           runCmd(ingressConfiguration),
		Resources:     loaders,
	}
}

func runCmd(ingressConfiguration *static.Configuration) func(_ []string) error {
	return func(_ []string) error {
		ingressConfiguration.SetEffectiveConfiguration()

		resp, errPing := Do(*ingressConfiguration)
		if resp != nil {
			resp.Body.Close()
		}
		if errPing != nil {
			fmt.Printf("Error calling healthcheck: %s\n", errPing)
			os.Exit(1)
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Bad healthcheck status: %s\n", resp.Status)
			os.Exit(1)
		}
		fmt.Printf("OK: %s\n", resp.Request.URL)
		os.Exit(0)
		return nil
	}
}

// Do try to do a healthcheck.
func Do(staticConfiguration static.Configuration) (*http.Response, error) {
	if staticConfiguration.Ping == nil {
		return nil, errors.New("please enable `ping` to use health check")
	}

	ep := staticConfiguration.Ping.EntryPoint
	if ep == "" {
		ep = "ingress"
	}

	pingEntryPoint, ok := staticConfiguration.EntryPoints[ep]
	if !ok {
		return nil, fmt.Errorf("ping: missing %s entry point", ep)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}
	protocol := "http"

	// TODO Handle TLS on ping etc...
	// if pingEntryPoint.TLS != nil {
	// 	protocol = "https"
	// 	tr := &http.Transport{
	// 		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	// 	}
	// 	client.Transport = tr
	// }

	path := "/"

	return client.Head(protocol + "://" + pingEntryPoint.GetAddress() + path + "ping")
}
