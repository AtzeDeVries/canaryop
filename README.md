# CanaryApp Opertaor

CanaryApp is an operator to handle canary deployments. 

* Handle component update
* Slowly send traffic to new deployment
* Cleanup old deployment if new deployment is ok
* Rollback if errors happen based on prometheus query

> Not yet ready for production. It is still restricted in configuration


## Overview

CanaryApp is based on the Kuberbuilder (v3) operator framework. It creates a 
deployment based on the image you provide. 


## Requirements

* Kubernetes cluster 1.16 or higher
* Istio 
* Prometheus

## Logic

The operator will try to get the requested state into the actual state. If you
update an image it will
* Deploy a secondary deployment with the new image
* Send some traffic to the secondary deployment using Istio
* Check with a prometheus query if the secondary deployment is ok
* Update weight a few times to give new image more load  
* If no errors and weight on new server > 50%
  * Update primary deployment
  * Send all traffic to primary deployment
  * Delete secondary deployment
* If errors
  * Send all traffic to primary deployment
  * Delete secondary deployment
  * Register failed image

For a more detailed view of the reconcile loop check this [Miro board](https://miro.com/app/board/o9J_lBbshxk=/)

## Deploying in a demo environment

### Cluster
This demo is based on a k3s Kubernetes cluster using [k3d](https://k3d.io/#installation)

```console
k3d cluster create canaryapp-demo \
  --port "8090:80@loadbalancer" \
  --agents 2 \
  --k3s-server-arg --disable=traefik

k3d kubeconfig merge  canaryapp-demo --kubeconfig-switch-context

kubectl get nodes
```
### Depenencies
Then we are going to install istio using [istioctl](https://istio.io/latest/docs/setup/getting-started/#download)
```console
istioctl install --set profile=demo -y
```
From the install directory of istio install prometheus
```console 
kubectl apply -f samples/addons/prometheus.yaml
```

### Operator
Deploy the CanaryApp operator
```console
kubectl kustomize config/default/ | kubectl apply -f -
```
### CanaryApp
We are now ready to install our app. Here is an example manifest

```yaml
apiVersion: canary.atze.io/v1
kind: CanaryApp
metadata:
  name: canaryapp-sample
spec:
  image: atzedevries/go-status-code-server:v1
  imageListenPort: 8088
  replicas: 2
  testReplicas: 1
  prometheusQuery: "increase(istio_requests_total{destination_service_name=\"canaryapp-sample\",response_code=~\"5.*\"}[20s]) > 2"
  prometheusURL: "http://prometheus.istio-system.svc:9090"
```

I've created a simple go web server which has two paths 
* `/hello` which returns a status 200
* `/nothello` which returns a status 500

There are three versions available, v1, v2 and v3. The sourcecode of this
app will be in the appendix. 

Apply the canaryapp-sample manifest and check if the pods are started. 

It is insightful to follow the logs of the operator, do this by
```console 
kubectl -n canaryop-system logs  deployments/canaryop-controller-manager -c manager -f
```

Update to a new version by 
```console 
kubectl patch canaryapp canaryapp-sample \
  --patch '{"spec": {"image": "atzedevries/go-status-code-server:v2" }}' \
  --type=merge
```

You can inspect the VirualService by. You see the weights being updated
```console 
kubectl get virtualservice canaryapp-sample -o yaml
```

You can also check the respone of the status-code server via
```console
curl localhost:8090/hello
```
It should return 'Hello! v1 (or v2 or v3)'

#### Introduce failure

Update to a new image using 
```console 
kubectl patch canaryapp canaryapp-sample \
  --patch '{"spec": {"image": "atzedevries/go-status-code-server:v3" }}' \
  --type=merge
```

Wait until the deployment is ready request this url a few times
```console
curl localhost:8090/nothello
```
This will generate a 500 http status. The operator will notice this via 
a prometheus query and start the rollback.


## Example Application

### Limit users only to allow using this resources using kuberntes rbac

We simulate this by making use of a service account. 
```console 
kubectl create serviceaccount limited-user
```
Create a role, rolebinding via
```console 
kubectl apply -f examples/limited-user/
```
Now using the service account you can see that the user is only limited to CRUD
in this namespace via 
```console
kubectl \
  --as=system:serviceaccount:default:limited-user \
  get canaryapps canaryapp-sample
```
But not allowed
```console
kubectl \
  --as=system:serviceaccount:default:limited-user \
  get pods
```

### Manage Resources via ArgoCD (GitOps)

Using this method the user only interfaces via git. 

First cleanup current CanaryApp instance in the default namespace
```console 
kubectl delete canaryapps canaryapp-sample
```
First we need to setup ArgoCD
```console 
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```
Readme about ArgoCD [here](https://argoproj.github.io/argo-cd/getting_started/)

Wait until all pods are started via checking 
```console 
kubectl get pods -n argocd
```

Once all pods are started get the initial password
```console
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d && echo
```
Create a port-foward to argocd interface
```console
kubectl port-forward svc/argocd-server -n argocd 8091:443
```
Go to https://localhost:8091 and login with admin and the password to
inspect the argocd interface.

#### Adding an app
Patch the this (or other) repository to argocd configmap
> if you want another repository, edit examples/argocd-app/project.yaml and add the repo
```console 
kubectl patch configmap/argocd-cm \
  -n argocd \
  --type merge \
  -p '{"data":{"repositories": "- name: canaryapp-demo\n  type: git\n  url: https://github.com/AtzeDeVries/canaryop"}}'
```

Apply an Argocd Project and App to sync
```console
kubectl apply -f examples/argocd-app
```

Changes to the git repository are synced to the ArgoCD app and the canaryapp is 
updated to reflect the changes. The user is only interacting with git and cannot modify
or destroy resources. This is limited by the app project. You can try to change
the namespace in `examples/argocd/canaryapp.yaml` and see what happens

## Cleanup

Cleanup the cluster
```console 
 k3d cluster delete canaryapp-demo
```

## Developing on CanaryApp operator

Files to check are
* `api/v1/canaryapp_types.go` for api description (run `make` and `make manifests` to apply update)
* `controllers/canaryapp_controller.go` for the reconcile loop.

To run the operator locally run `make install run`

To learn more about kubebuilder follow the [CronJob Tutorial](https://book.kubebuilder.io/cronjob-tutorial/cronjob-tutorial.html)

## Apendix

### Go status server source code
```go
package main

import (
	"fmt"
	"log"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Hello! v3")
}

func notHelloHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "NotHello! v3")
}


func main() {
	http.HandleFunc("/hello", helloHandler)
	http.HandleFunc("/nothello", notHelloHandler)

	fmt.Printf("Starting server at port 8088\n")
	if err := http.ListenAndServe(":8088", nil); err != nil {
		log.Fatal(err)
	}
}
```




