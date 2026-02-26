package gateway

import (
	"fmt"
	"strings"

	"github.com/hanzoai/ingress/v3/pkg/config/label"
)

const annotationsPrefix = "hanzo.ai/"

// ServiceConfig is the service's root configuration from annotations.
type ServiceConfig struct {
	Service Service `json:"service"`
}

// Service is the service's configuration from annotations.
type Service struct {
	NativeLB *bool `json:"nativeLB"`
}

func parseServiceAnnotations(annotations map[string]string) (ServiceConfig, error) {
	var svcConf ServiceConfig

	labels := convertAnnotations(annotations)
	if len(labels) == 0 {
		return svcConf, nil
	}

	if err := label.Decode(labels, &svcConf, "ingress.service."); err != nil {
		return svcConf, fmt.Errorf("decoding labels: %w", err)
	}

	return svcConf, nil
}

func convertAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return nil
	}

	result := make(map[string]string)

	for key, value := range annotations {
		if !strings.HasPrefix(key, annotationsPrefix) {
			continue
		}

		// Convert hanzo.ai/service.foo annotation to ingress.service.foo label format.
		newKey := "ingress." + strings.TrimPrefix(key, annotationsPrefix)
		result[newKey] = value
	}

	return result
}
