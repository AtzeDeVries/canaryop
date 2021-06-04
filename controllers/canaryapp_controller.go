/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"time"

	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istiogov1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	canaryv1 "github.com/atzedevries/canaryop/api/v1"
	"github.com/atzedevries/canaryop/resources"
)

// CanaryAppReconciler reconciles a CanaryApp object
type CanaryAppReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=canary.atze.io,resources=canaryapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=canary.atze.io,resources=canaryapps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=canary.atze.io,resources=canaryapps/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// Reconcile makes sure the requested state is the actual state. It is triggered by resources changes
// you decide to follow
func (r *CanaryAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("canaryop", req.NamespacedName)

	var canaryop canaryv1.CanaryApp
	if err := r.Get(ctx, req.NamespacedName, &canaryop); err != nil {
		log.Info("No Canaryop found")
		return ctrl.Result{}, nil
	}

	//log.Info("config", "config", canaryop.Spec)
	//os.Exit(1)
	// create VirtualService
	vs := resources.VirtualService(&canaryop)
	if err := r.deployVirtualService(ctx, canaryop, vs, log); err != nil {
		return ctrl.Result{}, err
	}

	// create DestinationRule
	dr := resources.DestinationRule(&canaryop)
	if err := r.deployDestinationRule(ctx, canaryop, dr, log); err != nil {
		return ctrl.Result{}, err
	}

	// create Gateway
	gw := resources.Gateway(&canaryop)
	if err := r.deployGateway(ctx, canaryop, gw, log); err != nil {
		return ctrl.Result{}, err
	}

	// Create Primary Deployment
	dep := resources.Deployment(&canaryop, "primary", canaryop.Spec.Replicas)
	if err := r.deployDeployment(ctx, canaryop, dep, log); err != nil {
		return ctrl.Result{}, err
	}

	// Create service
	svc := resources.Service(&canaryop)
	if err := r.deployService(ctx, canaryop, svc, log); err != nil {
		return ctrl.Result{}, err
	}

	// define secondary deployment for later usage
	depsec := resources.Deployment(&canaryop, "secondary", canaryop.Spec.TestReplicas)
	// handle the state of the new deployment
	if canaryop.Status.TestRunning {
		// requeue if Reconcile loop has been triggered to fast
		if time.Now().Sub(canaryop.Status.LastTrafficShift.Time).Seconds() < 30 {
			log.Info("Retriggering Reconcile loop because we have not waiting long enough to update traffic shift ")
			return ctrl.Result{RequeueAfter: time.Now().Sub(canaryop.Status.LastTrafficShift.Time)}, nil
		}

		// check if deployment has errors
		if hasDeploymentErrors(canaryop, log) {
			log.Info("Staring rollback")
			canaryop.Status.TestRunning = false
			canaryop.Status.SuccessfulRelease = false
			canaryop.Status.TrafficShift = 0
			canaryop.Status.LastFailedImage = canaryop.Spec.Image
			updateTrafficSplit(*vs, 100-canaryop.Status.TrafficShift)
			r.Status().Update(ctx, &canaryop)
			r.Update(ctx, vs)
			r.Delete(ctx, depsec)
			return ctrl.Result{}, nil
		}

		// update primary deployment to new version if trafficshift > 50%
		if canaryop.Status.TrafficShift >= 50 {
			for i, c := range dep.Spec.Template.Spec.Containers {
				if c.Name == canaryop.Name {
					dep.Spec.Template.Spec.Containers[i].Image = canaryop.Spec.Image
					log.Info("Updating primary deployment to new version")
					r.Update(ctx, dep)
					r.deploymentReady()
					log.Info("Updating traffic split to 100%")
					// update traffic shift status
					canaryop.Status.TestRunning = false
					canaryop.Status.SuccessfulRelease = true
					canaryop.Status.TrafficShift = 0
					updateTrafficSplit(*vs, 100-canaryop.Status.TrafficShift)
					r.Status().Update(ctx, &canaryop)
					r.Update(ctx, vs)
					log.Info("Deleting secondary deployment")
					r.Delete(ctx, depsec)
					return ctrl.Result{}, nil
				}
			}
		}
		log.Info("Updating traffic split to next value")
		log.Info("Found", "trafficsplit", canaryop.Status.TrafficShift)
		canaryop.Status.TrafficShift += 10
		log.Info("Updated to", "trafficsplit", canaryop.Status.TrafficShift)
		updateTrafficSplit(*vs, 100-canaryop.Status.TrafficShift)
		canaryop.Status.LastTrafficShift = &metav1.Time{Time: time.Now()}
		r.Status().Update(ctx, &canaryop)
		r.Update(ctx, vs)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if versions is updated
	for _, c := range dep.Spec.Template.Spec.Containers {
		if canaryop.Spec.Image != c.Image {
			log.Info("Found new tag")
			if canaryop.Spec.Image == canaryop.Status.LastFailedImage {
				log.Info("Current set tag is a failed tag")
				return ctrl.Result{}, nil
			}

			if err := r.deployDeployment(ctx, canaryop, depsec, log); err != nil {
				return ctrl.Result{}, err
			}
			r.deploymentReady()
			canaryop.Status.TestRunning = true
			canaryop.Status.TrafficShift = 10
			updateTrafficSplit(*vs, 100-canaryop.Status.TrafficShift)
			canaryop.Status.LastTrafficShift = &metav1.Time{Time: time.Now()}
			// reflect in status
			r.Status().Update(ctx, &canaryop)
			r.Update(ctx, vs)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CanaryAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//https://github.com/kubernetes-sigs/kubebuilder/issues/618#issuecomment-698018831
	pred := predicate.GenerationChangedPredicate{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&canaryv1.CanaryApp{}).
		WithEventFilter(pred).
		Complete(r)
}

func (r *CanaryAppReconciler) deploymentReady() {
	// TODO: implement deployment ready code
	// check https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/kubectl/pkg/polymorphichelpers/rollout_status.go#L75
	log := r.Log.WithName("canaryapp")
	log.Info("Waiting for deployment not yet implemented, just waiting for 20 seconds")
	time.Sleep(time.Second * 20)
	return
}

func (r *CanaryAppReconciler) deployVirtualService(ctx context.Context, c canaryv1.CanaryApp, deploy *istiogov1alpha3.VirtualService, log logr.Logger) error {
	err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: c.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(&c, deploy, r.Scheme)
		log.Info("Deploying", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		err = r.Create(ctx, deploy)
		if err != nil {
			log.Info("Failed to create", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		return err
	}
	return nil
}

func (r *CanaryAppReconciler) deployGateway(ctx context.Context, c canaryv1.CanaryApp, deploy *istiogov1alpha3.Gateway, log logr.Logger) error {
	err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: c.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(&c, deploy, r.Scheme)
		log.Info("Deploying", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		err = r.Create(ctx, deploy)
		if err != nil {
			log.Info("Failed to create", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		return err
	}
	return nil
}

func (r *CanaryAppReconciler) deployDestinationRule(ctx context.Context, c canaryv1.CanaryApp, deploy *istiogov1alpha3.DestinationRule, log logr.Logger) error {
	err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: c.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(&c, deploy, r.Scheme)
		log.Info("Deploying", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		err = r.Create(ctx, deploy)
		if err != nil {
			log.Info("Failed to create", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		return err
	}
	return nil
}

func (r *CanaryAppReconciler) deployService(ctx context.Context, c canaryv1.CanaryApp, deploy *corev1.Service, log logr.Logger) error {
	err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: c.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(&c, deploy, r.Scheme)
		log.Info("Deploying", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		err = r.Create(ctx, deploy)
		if err != nil {
			log.Info("Failed to create", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		return err
	}
	return nil
}

func (r *CanaryAppReconciler) deployDeployment(ctx context.Context, c canaryv1.CanaryApp, deploy *appsv1.Deployment, log logr.Logger) error {
	err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: c.Namespace}, deploy)
	if err != nil && errors.IsNotFound(err) {
		ctrl.SetControllerReference(&c, deploy, r.Scheme)
		log.Info("Deploying", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		err = r.Create(ctx, deploy)
		if err != nil {
			log.Info("Failed to create", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
			return err
		}
	} else if err != nil {
		log.Error(err, "Failed to get", deploy.Kind, deploy.Namespace+"/"+deploy.Name)
		return err
	}
	return nil
}

func updateTrafficSplit(vs istiogov1alpha3.VirtualService, weight int32) {
	vs.Spec.Http[0] = &istiov1alpha3.HTTPRoute{
		Match: []*istiov1alpha3.HTTPMatchRequest{{}},
		Route: []*istiov1alpha3.HTTPRouteDestination{{
			Weight: weight,
			Destination: &istiov1alpha3.Destination{
				Host:   vs.Name,
				Subset: "primary",
				Port: &istiov1alpha3.PortSelector{
					Number: 8088,
				},
			},
		},
			{
				Weight: 100 - weight,
				Destination: &istiov1alpha3.Destination{
					Host:   vs.Name,
					Subset: "secondary",
					Port: &istiov1alpha3.PortSelector{
						Number: 8088,
					},
				},
			}},
	}
}

func hasDeploymentErrors(c canaryv1.CanaryApp, log logr.Logger) bool {
	client, err := promapi.NewClient(promapi.Config{
		Address: c.Spec.PrometheusURL,
	})
	if err != nil {
		log.Error(err, "Error creating client")

	}
	v1api := promv1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := promv1.Range{
		Start: time.Now().Add(-2 * time.Minute),
		End:   time.Now(),
		Step:  5 * time.Second,
	}
	result, warnings, err := v1api.QueryRange(ctx, c.Spec.PrometheusQuery, r)
	if err != nil {
		log.Error(err, "Error querying Prometheus")
	}
	if len(warnings) > 0 {
		log.V(3).Info("Warnings:", warnings)
	}
	log.Info("Query", "result", result)
	if result.String() != "" {
		log.Info("Found some issues Friend! ")
		return true
	}
	return false

}
