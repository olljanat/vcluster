#!/usr/bin/env bash

tools1pod=$(kubectl --context kind-host-cluster-1 get pods -l app=tools -o name)
tools1ip=$(kubectl --context kind-host-cluster-1 get $tools1pod -o jsonpath="{.status.podIP}")
tools2pod=$(kubectl --context kind-host-cluster-2 get pods -l app=tools -o name)
tools2ip=$(kubectl --context kind-host-cluster-2 get $tools2pod -o jsonpath="{.status.podIP}")
tools3pod=$(kubectl --context kind-host-cluster-3 get pods -l app=tools -o name)
tools3ip=$(kubectl --context kind-host-cluster-3 get $tools3pod -o jsonpath="{.status.podIP}")

echo
echo "Testing ping from $tools1pod ($tools1ip) to $tools2pod ($tools2ip)"
kubectl --context kind-host-cluster-1 exec -it $tools1pod -- ping -c 1 $tools2ip
echo
echo "Testing ping from $tools2pod ($tools2ip) to $tools3pod ($tools3ip)"
kubectl --context kind-host-cluster-2 exec -it $tools2pod -- ping -c 1 $tools3ip
echo
echo "Testing ping from $tools3pod ($tools3ip) to $tools1pod ($tools1ip)"
kubectl --context kind-host-cluster-3 exec -it $tools3pod -- ping -c 1 $tools1ip
