package resources

import (
	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istiogov1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualService defines a (istio) VirtualService for canaryop
func VirtualService(c *canaryv1.CanaryApp) *istiogov1alpha3.VirtualService {
	vs := &istiogov1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualService",
			APIVersion: "networking.istio.io/v1alpha3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
		Spec: istiov1alpha3.VirtualService{
			Hosts:    []string{"*"},
			Gateways: []string{c.Name},
			Http: []*istiov1alpha3.HTTPRoute{{
				Match: []*istiov1alpha3.HTTPMatchRequest{{}},
				Route: []*istiov1alpha3.HTTPRouteDestination{{
					Weight: 100,
					Destination: &istiov1alpha3.Destination{
						Host:   c.Name,
						Subset: "primary",
						Port: &istiov1alpha3.PortSelector{
							Number: uint32(c.Spec.ImageListenPort),
						},
					},
				},
					{
						Weight: 0,
						Destination: &istiov1alpha3.Destination{
							Host:   c.Name,
							Subset: "secondary",
							Port: &istiov1alpha3.PortSelector{
								Number: uint32(c.Spec.ImageListenPort),
							},
						},
					}},
			}},
		},
	}
	return vs
}
