package resources

import (
	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istiogov1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Gateway defines a (istio) gateway for canaryop
func Gateway(c *canaryv1.CanaryApp) *istiogov1alpha3.Gateway {
	ds := &istiogov1alpha3.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "networking.istio.io/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
		Spec: istiov1alpha3.Gateway{
			Selector: map[string]string{"istio": "ingressgateway"},
			Servers: []*istiov1alpha3.Server{{
				Hosts: []string{"*"},
				Port: &istiov1alpha3.Port{
					Number:   80,
					Name:     "http",
					Protocol: "HTTP",
				},
			}},
		},
	}
	return ds
}
