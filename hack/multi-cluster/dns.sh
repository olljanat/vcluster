#!/usr/bin/env bash

echo
echo "Configuring core DNS"
coredns1=$(kubectl --context kind-host-cluster-1 -n kube-system get pods -l k8s-app=kube-dns -o jsonpath="{.items[*].status.podIP}")
coredns2=$(kubectl --context kind-host-cluster-2 -n kube-system get pods -l k8s-app=kube-dns -o jsonpath="{.items[*].status.podIP}")
coredns3=$(kubectl --context kind-host-cluster-3 -n kube-system get pods -l k8s-app=kube-dns -o jsonpath="{.items[*].status.podIP}")

cat << EOF > temp-coredns1.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    host-cluster2.local {
        forward . $coredns2
        log
    }
    host-cluster3.local {
        forward . $coredns3
        log
    }
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes host-cluster1.local cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
           except host-cluster2.local host-cluster3.local
        }
        cache 30
        loop
        reload
        loadbalance
    }
EOF
cat << EOF > temp-coredns2.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    host-cluster1.local {
        forward . $coredns1
        log
    }
    host-cluster3.local {
        forward . $coredns3
        log
    }
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes host-cluster2.local cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
           except host-cluster1.local host-cluster3.local
        }
        cache 30
        loop
        reload
        loadbalance
    }
EOF
cat << EOF > temp-coredns3.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    host-cluster1.local {
        forward . $coredns1
        log
    }
    host-cluster2.local {
        forward . $coredns2
        log
    }
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes host-cluster3.local cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
           except host-cluster1.local host-cluster2.local
        }
        cache 30
        loop
        reload
        loadbalance
    }
EOF

kubectl --context kind-host-cluster-1 -n kube-system apply -f temp-coredns1.yaml
kubectl --context kind-host-cluster-2 -n kube-system apply -f temp-coredns2.yaml
kubectl --context kind-host-cluster-3 -n kube-system apply -f temp-coredns3.yaml

echo
echo "Starting test services"
kubectl --context kind-host-cluster-1 create deployment web --image=bitnami/nginx
kubectl --context kind-host-cluster-1 apply -f service_web.yaml
kubectl --context kind-host-cluster-2 create deployment web --image=bitnami/nginx
kubectl --context kind-host-cluster-2 apply -f service_web.yaml
kubectl --context kind-host-cluster-3 create deployment web --image=bitnami/nginx
kubectl --context kind-host-cluster-3 apply -f service_web.yaml


calico1pod=$(kubectl --context kind-host-cluster-1 -n kube-system get pods -l k8s-app=calico-node -o name)
calico2pod=$(kubectl --context kind-host-cluster-2 -n kube-system get pods -l k8s-app=calico-node -o name)
calico3pod=$(kubectl --context kind-host-cluster-3 -n kube-system get pods -l k8s-app=calico-node -o name)
