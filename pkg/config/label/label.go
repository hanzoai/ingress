// Package label implements the decoding and encoding between flat labels and a typed Configuration.
package label

import (
	"github.com/traefik/paerser/parser"
	"github.com/hanzoai/ingress/v3/pkg/config/dynamic"
)

// RootName is the root name used as prefix for labels and configuration keys.
const RootName = "ingress"

// DecodeConfiguration converts the labels to a configuration.
func DecodeConfiguration(labels map[string]string) (*dynamic.Configuration, error) {
	conf := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{},
		TCP:  &dynamic.TCPConfiguration{},
		UDP:  &dynamic.UDPConfiguration{},
		TLS:  &dynamic.TLSConfiguration{},
	}

	// When decoding the TLS configuration we are making sure that only the default TLS store can be configured.
	err := parser.Decode(labels, conf, RootName, "ingress.http", "ingress.tcp", "ingress.udp", "ingress.tls.stores.default")
	if err != nil {
		return nil, err
	}

	return conf, nil
}

// EncodeConfiguration converts a configuration to labels.
func EncodeConfiguration(conf *dynamic.Configuration) (map[string]string, error) {
	return parser.Encode(conf, RootName)
}

// Decode converts the labels to an element.
// labels -> [ node -> node + metadata (type) ] -> element (node).
func Decode(labels map[string]string, element any, filters ...string) error {
	return parser.Decode(labels, element, RootName, filters...)
}
