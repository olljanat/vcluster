#!/usr/bin/env bash

kube1ip=$(kubectl --context kind-host-cluster-1 get endpoints kubernetes -o jsonpath="{.subsets[*].addresses[*].ip}")
kube2ip=$(kubectl --context kind-host-cluster-2 get endpoints kubernetes -o jsonpath="{.subsets[*].addresses[*].ip}")
kube3ip=$(kubectl --context kind-host-cluster-3 get endpoints kubernetes -o jsonpath="{.subsets[*].addresses[*].ip}")

cat << EOF > temp-calico-endpoint1.yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: kubernetes-services-endpoint
  namespace: kube-system
data:
  KUBERNETES_SERVICE_HOST: "$kube1ip"
  KUBERNETES_SERVICE_PORT: "6443"
EOF
cat << EOF > temp-calico-endpoint2.yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: kubernetes-services-endpoint
  namespace: kube-system
data:
  KUBERNETES_SERVICE_HOST: "$kube2ip"
  KUBERNETES_SERVICE_PORT: "6443"
EOF
cat << EOF > temp-calico-endpoint3.yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: kubernetes-services-endpoint
  namespace: kube-system
data:
  KUBERNETES_SERVICE_HOST: "$kube3ip"
  KUBERNETES_SERVICE_PORT: "6443"
EOF
kubectl --context kind-host-cluster-1 apply -f temp-calico-endpoint1.yaml
kubectl --context kind-host-cluster-2 apply -f temp-calico-endpoint2.yaml
kubectl --context kind-host-cluster-3 apply -f temp-calico-endpoint3.yaml

sleep 60s

kubectl --context kind-host-cluster-1 delete pod -n kube-system -l k8s-app=calico-node
kubectl --context kind-host-cluster-1 delete pod -n kube-system -l k8s-app=calico-kube-controllers
kubectl --context kind-host-cluster-1 delete pod -n kube-system -l k8s-app=kube-dns

kubectl --context kind-host-cluster-2 delete pod -n kube-system -l k8s-app=calico-node
kubectl --context kind-host-cluster-2 delete pod -n kube-system -l k8s-app=calico-kube-controllers
kubectl --context kind-host-cluster-2 delete pod -n kube-system -l k8s-app=kube-dns

kubectl --context kind-host-cluster-3 delete pod -n kube-system -l k8s-app=calico-node
kubectl --context kind-host-cluster-3 delete pod -n kube-system -l k8s-app=calico-kube-controllers
kubectl --context kind-host-cluster-3 delete pod -n kube-system -l k8s-app=kube-dns

sleep 60s

