#!/usr/bin/env bash

calico1ip=$(kubectl --context kind-host-cluster-1 -n kube-system get pods -l k8s-app=calico-node -o jsonpath="{.items[*].status.podIP}")
calico2ip=$(kubectl --context kind-host-cluster-2 -n kube-system get pods -l k8s-app=calico-node -o jsonpath="{.items[*].status.podIP}")
calico3ip=$(kubectl --context kind-host-cluster-3 -n kube-system get pods -l k8s-app=calico-node -o jsonpath="{.items[*].status.podIP}")


cat << EOF > temp-calico-endpoint1.yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: kubernetes-services-endpoint
  namespace: kube-system
data:
  KUBERNETES_SERVICE_HOST: "$calico1ip"
  KUBERNETES_SERVICE_PORT: "6443"
EOF
cat << EOF > temp-calico-endpoint2.yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: kubernetes-services-endpoint
  namespace: kube-system
data:
  KUBERNETES_SERVICE_HOST: "$calico2ip"
  KUBERNETES_SERVICE_PORT: "6443"
EOF
cat << EOF > temp-calico-endpoint3.yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: kubernetes-services-endpoint
  namespace: kube-system
data:
  KUBERNETES_SERVICE_HOST: "$calico3ip"
  KUBERNETES_SERVICE_PORT: "6443"
EOF
kubectl --context kind-host-cluster-1 apply -f temp-calico-endpoint1.yaml
kubectl --context kind-host-cluster-2 apply -f temp-calico-endpoint2.yaml
kubectl --context kind-host-cluster-3 apply -f temp-calico-endpoint3.yaml

kubectl delete pod -n kube-system -l k8s-app=calico-node
kubectl delete pod -n kube-system -l k8s-app=calico-kube-controllers
