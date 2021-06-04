package resources

import (
	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istiogov1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DestinationRule defines a (istio) destination rule for canaryop
func DestinationRule(c *canaryv1.CanaryApp) *istiogov1alpha3.DestinationRule {
	ds := &istiogov1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DestinationRule",
			APIVersion: "networking.istio.io/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
		Spec: istiov1alpha3.DestinationRule{
			Host: c.Name,
			Subsets: []*istiov1alpha3.Subset{{
				Name:   "primary",
				Labels: map[string]string{"app": c.Name, "service": "primary"},
			},
				{
					Name:   "secondary",
					Labels: map[string]string{"app": c.Name, "service": "secondary"},
				}},
		},
	}
	return ds
}
