package defaultsecuritycontext

import (
	"context"
	"fmt"

	"github.com/loft-sh/vcluster/pkg/util/loghelper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilpointer "k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type DefaultSecurityContextWebhook struct {
	client.Client
	DefaultSecurityContextStandard string
	Log                            loghelper.Logger
}

var _ admission.CustomDefaulter = &DefaultSecurityContextWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DefaultSecurityContextWebhook) Default(ctx context.Context, obj runtime.Object) error {
	pod := obj.(*corev1.Pod)

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
			r.Log.Infof(`add default security context to container "%s on pod %s"`, container.Name, pod.Name)
		}
	}
	return nil
}

/*
func (r *DefaultSecurityContextWebhook) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
*/

func (r *DefaultSecurityContextWebhook) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(&DefaultSecurityContextWebhook{}).
		Complete()
	if err != nil {
		return fmt.Errorf("unable to setup default security context controller: %v", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check for default security context controller: %v", err)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check for default security context controller: %v", err)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("unable to run manager for default security context controller: %v", err)
	}
	return nil
}
