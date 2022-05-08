package podsecurity

import (
	"context"
	"time"

	"github.com/loft-sh/vcluster/pkg/util/loghelper"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/pod-security-admission/api"
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodSecurityReconciler struct {
	client.Client
	PodSecurityStandard string
	Log                 loghelper.Logger
}

func (r *PodSecurityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	client := r.Client

	r.Log.Infof(`DEBUG resource %s"`, req.NamespacedName)

	ns := &corev1.Namespace{}
	err := client.Get(ctx, req.NamespacedName, ns)
	isNamespace := true
	if err != nil {
		if kerrors.IsNotFound(err) {
			isNamespace = false
		} else {
			return ctrl.Result{RequeueAfter: time.Second}, err
		}
	}

	pod := &corev1.Pod{}
	err = client.Get(ctx, req.NamespacedName, pod)
	isPod := true
	if err != nil {
		if kerrors.IsNotFound(err) {
			isPod = false
		} else {
			return ctrl.Result{RequeueAfter: time.Second}, err
		}
	}

	if isNamespace == true {
		labels := ns.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}

		if v, ok := labels[api.EnforceLevelLabel]; !ok || v != r.PodSecurityStandard {
			labels[api.EnforceLevelLabel] = r.PodSecurityStandard
			labels[api.EnforceVersionLabel] = api.VersionLatest
			labels[api.WarnLevelLabel] = r.PodSecurityStandard
			labels[api.WarnVersionLabel] = api.VersionLatest
			ns.SetLabels(labels)
			err = client.Update(ctx, ns)
			if err != nil {
				return ctrl.Result{RequeueAfter: time.Second}, err
			}
			r.Log.Infof(`enforcing pod security standard "%s" on namespace "%s"`, r.PodSecurityStandard, ns.Name)
		}
	}

	if isPod == true {
		r.Log.Infof(`DEBUG pod %s"`, pod.Name)

		securityContextAdded := false
		for i, container := range pod.Spec.Containers {
			if container.SecurityContext == nil {
				pod.Spec.Containers[i].SecurityContext = &corev1.SecurityContext{
					AllowPrivilegeEscalation: utilpointer.Bool(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
						Add:  []corev1.Capability{"NET_BIND_SERVICE"},
					},
					RunAsNonRoot: utilpointer.Bool(false),
					RunAsUser:    utilpointer.Int64(65534),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				}
				securityContextAdded = true
				r.Log.Infof(`add default security context to container "%s on pod %s"`, container.Name, pod.Name)
			}
		}
		if securityContextAdded == true {
			r.Log.Infof(`updating pod %s for security context"`, pod.Name)
			err = client.Update(ctx, pod)
			if err != nil {
				return ctrl.Result{RequeueAfter: time.Second}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager adds the controller to the manager
func (r *PodSecurityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pod_security").
		For(&corev1.Namespace{}).
		Complete(r)
}
