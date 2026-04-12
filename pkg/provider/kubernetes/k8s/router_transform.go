package k8s

import (
	"context"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"k8s.io/apimachinery/pkg/runtime"
)

type RouterTransform interface {
	Apply(ctx context.Context, rt *dynamic.Router, object runtime.Object) error
}
