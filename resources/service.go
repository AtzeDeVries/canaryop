package resources

import (
	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service defines a service for canaryop
func Service(c *canaryv1.CanaryApp) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Port: 8088,
				Name: "http",
			}},
			Selector: map[string]string{"app": c.Name},
		},
	}
	return svc
}
