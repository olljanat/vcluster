#!/usr/bin/env bash

echo
echo "Creating clusters"
kind create cluster --config kind-cluster1.yaml
kubectl --context kind-host-cluster-1 apply -f https://docs.projectcalico.org/v3.22/manifests/calico.yaml

kind create cluster --config kind-cluster2.yaml
kubectl --context kind-host-cluster-2 apply -f https://docs.projectcalico.org/v3.22/manifests/calico.yaml

kind create cluster --config kind-cluster3.yaml
kubectl --context kind-host-cluster-3 apply -f https://docs.projectcalico.org/v3.22/manifests/calico.yaml

kubectl --context kind-host-cluster-1 apply -f calico-felix-config.yaml
kubectl --context kind-host-cluster-2 apply -f calico-felix-config.yaml
kubectl --context kind-host-cluster-3 apply -f calico-felix-config.yaml

./bpf.sh

echo
echo "Setting global BGP configuration to Calico"
cat << EOF > temp-bgp-config-1.yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPConfiguration
metadata:
  name: default
spec:
  asNumber: 65001
  serviceClusterIPs:
  - cidr: 10.96.0.0/12
EOF
cat << EOF > temp-bgp-config-2.yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPConfiguration
metadata:
  name: default
spec:
  asNumber: 65002
  serviceClusterIPs:
  - cidr: 10.96.0.0/12
EOF
cat << EOF > temp-bgp-config-3.yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPConfiguration
metadata:
  name: default
spec:
  asNumber: 65003
  serviceClusterIPs:
  - cidr: 10.96.0.0/12
EOF
kubectl --context kind-host-cluster-1 apply -f temp-bgp-config-1.yaml
kubectl --context kind-host-cluster-2 apply -f temp-bgp-config-2.yaml
kubectl --context kind-host-cluster-3 apply -f temp-bgp-config-3.yaml
sleep 30s

echo
echo "Creating BGP peering"
calico1ip=$(kubectl --context kind-host-cluster-1 -n kube-system get pods -l k8s-app=calico-node -o jsonpath="{.items[*].status.podIP}")
calico2ip=$(kubectl --context kind-host-cluster-2 -n kube-system get pods -l k8s-app=calico-node -o jsonpath="{.items[*].status.podIP}")
calico3ip=$(kubectl --context kind-host-cluster-3 -n kube-system get pods -l k8s-app=calico-node -o jsonpath="{.items[*].status.podIP}")

cat << EOF > temp-bgp-peer-1.yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPPeer
metadata:
  name: cluster-1
spec:
  peerIP: $calico1ip
  asNumber: 65001
EOF
cat << EOF > temp-bgp-peer-2.yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPPeer
metadata:
  name: cluster-2
spec:
  peerIP: $calico2ip
  asNumber: 65002
EOF
cat << EOF > temp-bgp-peer-3.yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPPeer
metadata:
  name: cluster-3
spec:
  peerIP: $calico3ip
  asNumber: 65003
EOF
kubectl --context kind-host-cluster-1 apply -f temp-bgp-peer-2.yaml -f temp-bgp-peer-3.yaml
kubectl --context kind-host-cluster-2 apply -f temp-bgp-peer-1.yaml -f temp-bgp-peer-3.yaml
kubectl --context kind-host-cluster-3 apply -f temp-bgp-peer-1.yaml -f temp-bgp-peer-2.yaml

echo
echo "Starting tools containers"
kubectl --context kind-host-cluster-1 create deployment tools --image=busybox -- sleep infinity
kubectl --context kind-host-cluster-2 create deployment tools --image=busybox -- sleep infinity
kubectl --context kind-host-cluster-3 create deployment tools --image=busybox -- sleep infinity
sleep 30s

./dns.sh

./ping.sh
