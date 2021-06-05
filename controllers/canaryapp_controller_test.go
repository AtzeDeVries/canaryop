package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"time"

	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	istiogov1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
)

var _ = Describe("Canaryapp controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		CanaryAppName                    = "test-canaryapp"
		CanaryAppNamespace               = "default"
		ImageV1                          = "atzedevries/go-status-code-server:v1"
		ImageV2                          = "atzedevries/go-status-code-server:v2"
		PrimaryReplicas                  = 2
		TestReplicas                     = 1
		InitialTrafficShift        int32 = 10
		servicePort                int32 = 8088
		DeploymentReadyWaitTime    int32 = 1
		TrafficShiftUpdateInterval int32 = 1

		timeout  = time.Second * 20
		duration = time.Second * 10
		interval = time.Millisecond * 250
		//resourceCreateTimeout = time.Second * 5
	)

	Context("When updating the image tag", func() {
		// cannot address const so create vars here
		//var PrimaryReplicas int32 = 2
		//var TestReplicas int32 = 2
		It("It should test and update to the new image tag", func() {
			By("By creating a new CanaryApp")
			ctx := context.Background()
			canaryapp := &canaryv1.CanaryApp{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "canary.atze.io/v1",
					Kind:       "CanaryApp",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      CanaryAppName,
					Namespace: CanaryAppNamespace,
				},
				Spec: canaryv1.CanaryAppSpec{
					Image:                      ImageV1,
					Replicas:                   PrimaryReplicas,
					TestReplicas:               TestReplicas,
					PrometheusQuery:            "",
					PrometheusURL:              "",
					FailWhenPrometheusFails:    false,
					DeploymentReadyWaitTime:    DeploymentReadyWaitTime,
					TrafficShiftUpdateInterval: TrafficShiftUpdateInterval,
				},
			}
			Expect(k8sClient.Create(ctx, canaryapp)).Should(Succeed())

			canaryappLookupKey := types.NamespacedName{Name: CanaryAppName, Namespace: CanaryAppNamespace}
			createdCanaryApp := &canaryv1.CanaryApp{}

			// We'll need to retry getting this newly created CanaryApp, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryappLookupKey, createdCanaryApp)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(createdCanaryApp.Spec.Image).Should(Equal(ImageV1))

			By("By checking the CanarayApp for testStatus")
			Consistently(func() (bool, error) {
				err := k8sClient.Get(ctx, canaryappLookupKey, createdCanaryApp)
				if err != nil {
					return true, err
				}
				return createdCanaryApp.Status.TestRunning, nil
			}, duration, interval).Should(Equal(false))

			By("By checking the creation of the Primary Deployment")
			primaryDeploymentLookupKey := types.NamespacedName{Name: CanaryAppName + "-primary", Namespace: CanaryAppNamespace}
			createdPrimaryDeployment := &appsv1.Deployment{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, primaryDeploymentLookupKey, createdPrimaryDeployment)
				if err != nil {
					return ImageV2
				}
				return createdPrimaryDeployment.Spec.Template.Spec.Containers[0].Image
			}, timeout, interval).Should(Equal(ImageV1))

			By("By checking the creation of the VirtualService")
			virtualServiceLookupKey := types.NamespacedName{Name: CanaryAppName, Namespace: CanaryAppNamespace}
			createdVirtualService := &istiogov1alpha3.VirtualService{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, virtualServiceLookupKey, createdVirtualService)
				if err != nil {
					return ""
				}
				return createdVirtualService.Spec.Gateways[0]
			}, timeout, interval).Should(Equal(CanaryAppName))

			By("By checking the creation of the Gateway")
			gatewayLookupKey := types.NamespacedName{Name: CanaryAppName, Namespace: CanaryAppNamespace}
			createdGateway := &istiogov1alpha3.Gateway{}
			Eventually(func() map[string]string {
				err := k8sClient.Get(ctx, gatewayLookupKey, createdGateway)
				if err != nil {
					return map[string]string{CanaryAppNamespace: CanaryAppName}
				}
				return createdGateway.Spec.Selector
			}, timeout, interval).Should(Equal(map[string]string{"istio": "ingressgateway"}))

			By("By checking the creation of the DestinationRule")
			destinationRuleLookupKey := types.NamespacedName{Name: CanaryAppName, Namespace: CanaryAppNamespace}
			createdDestinationRule := &istiogov1alpha3.DestinationRule{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, destinationRuleLookupKey, createdDestinationRule)
				if err != nil {
					return ""
				}
				return createdDestinationRule.Spec.Host
			}, timeout, interval).Should(Equal(CanaryAppName))

			By("By checking the creation of the Service")
			ServiceLookupKey := types.NamespacedName{Name: CanaryAppName, Namespace: CanaryAppNamespace}
			createdService := &corev1.Service{}
			Eventually(func() int32 {
				err := k8sClient.Get(ctx, ServiceLookupKey, createdService)
				if err != nil {
					return 0
				}
				return createdService.Spec.Ports[0].Port
			}, timeout, interval).Should(Equal(servicePort))

			By("By updating canaryapp with a new image to trigger a test")
			canaryapp.Spec.Image = ImageV2
			Expect(k8sClient.Update(ctx, canaryapp)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryappLookupKey, createdCanaryApp)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(createdCanaryApp.Spec.Image).Should(Equal(ImageV2))

			secondaryDeploymentLookupKey := types.NamespacedName{Name: CanaryAppName + "-secondary", Namespace: CanaryAppNamespace}
			createdSecondaryDeployment := &appsv1.Deployment{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, secondaryDeploymentLookupKey, createdSecondaryDeployment)
				if err != nil {
					return ImageV1
				}
				return createdSecondaryDeployment.Spec.Template.Spec.Containers[0].Image
			}, timeout, interval).Should(Equal(ImageV2))

			By("Waiting for the deployment to update the test running state")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryappLookupKey, createdCanaryApp)
				if err != nil {
					return false
				}
				return createdCanaryApp.Status.TestRunning
			}, timeout, interval).Should(BeTrue())
			Expect(createdCanaryApp.Status.TrafficShift).Should(Equal(InitialTrafficShift))

			By("Waiting for the next reconcile the trafficshift should be updated")
			Eventually(func() int32 {
				err := k8sClient.Get(ctx, canaryappLookupKey, createdCanaryApp)
				if err != nil {
					return -1
				}
				return createdCanaryApp.Status.TrafficShift
			}, timeout, interval).Should(Equal(InitialTrafficShift + 10))

			By("Waiting for next reconcile with TrafficShift > 50 update of primary should be done ")
			Eventually(func() string {
				err := k8sClient.Get(ctx, primaryDeploymentLookupKey, createdPrimaryDeployment)
				if err != nil {
					return ImageV1
				}
				return createdPrimaryDeployment.Spec.Template.Spec.Containers[0].Image
			}, timeout, interval).Should(Equal(ImageV2))

		})
	})
})
