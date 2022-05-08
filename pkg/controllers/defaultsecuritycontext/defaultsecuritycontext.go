package defaultsecuritycontext

import (
	"context"
	"time"

	"github.com/loft-sh/vcluster/pkg/util/loghelper"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DefaultSecurityContextReconciler struct {
	client.Client
	DefaultSecurityContextStandard string
	Log                            loghelper.Logger
}

func (r *DefaultSecurityContextReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	client := r.Client
	pod := &corev1.Pod{}
	err := client.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: time.Second}, err
	}

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

	return ctrl.Result{}, nil
}

// SetupWithManager adds the controller to the manager
func (r *DefaultSecurityContextReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("pod_security").
		For(&corev1.Pod{}).
		Complete(r)
}
