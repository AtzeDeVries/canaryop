package resources

import (
	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployment defines a deployment for canaryop
func Deployment(c *canaryv1.CanaryApp, service string, replicas int32) *appsv1.Deployment {
	ls := labelsForDeployment(c.Name, service)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name + "-" + service,
			Namespace: c.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: c.Spec.Image,
						Name:  c.Name,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8088,
							Name:          "http",
						}},
					}},
				},
			},
		},
	}

	return dep
}

func labelsForDeployment(name string, service string) map[string]string {
	return map[string]string{"app": name, "owner": "canaryop", "service": service}

}
