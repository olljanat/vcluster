package controllers

import (
	"fmt"
	"strings"

	"github.com/loft-sh/vcluster/pkg/controllers/k8sdefaultendpoint"
	"github.com/loft-sh/vcluster/pkg/controllers/manifests"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/serviceaccounts"

	"github.com/loft-sh/vcluster/cmd/vcluster/context"
	"github.com/loft-sh/vcluster/pkg/controllers/coredns"
	"github.com/loft-sh/vcluster/pkg/controllers/defaultsecuritycontext"
	"github.com/loft-sh/vcluster/pkg/controllers/podsecurity"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/configmaps"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/endpoints"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/events"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/ingresses"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/networkpolicies"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/nodes"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/persistentvolumeclaims"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/persistentvolumes"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/poddisruptionbudgets"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/pods"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/priorityclasses"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/secrets"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/services"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/storageclasses"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/volumesnapshots/volumesnapshotclasses"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/volumesnapshots/volumesnapshotcontents"
	"github.com/loft-sh/vcluster/pkg/controllers/resources/volumesnapshots/volumesnapshots"
	"github.com/loft-sh/vcluster/pkg/controllers/syncer"
	synccontext "github.com/loft-sh/vcluster/pkg/controllers/syncer/context"
	"github.com/loft-sh/vcluster/pkg/util/loghelper"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var ResourceControllers = map[string][]func(*synccontext.RegisterContext) (syncer.Object, error){
	"services":               newControllers(services.New),
	"configmaps":             newControllers(configmaps.New),
	"secrets":                newControllers(secrets.New),
	"endpoints":              newControllers(endpoints.New),
	"pods":                   newControllers(pods.New),
	"events":                 newControllers(events.New),
	"persistentvolumeclaims": newControllers(persistentvolumeclaims.New),
	"ingresses":              newControllers(ingresses.New),
	"storageclasses":         newControllers(storageclasses.New),
	"legacy-storageclasses":  newControllers(storageclasses.NewLegacy),
	"priorityclasses":        newControllers(priorityclasses.New),
	"nodes,fake-nodes":       newControllers(nodes.New),
	"poddisruptionbudgets":   newControllers(poddisruptionbudgets.New),
	"networkpolicies":        newControllers(networkpolicies.New),
	"volumesnapshots":        newControllers(volumesnapshotclasses.New, volumesnapshots.New, volumesnapshotcontents.New),
	"serviceaccounts":        newControllers(serviceaccounts.New),
	"persistentvolumes,fake-persistentvolumes": newControllers(persistentvolumes.New),
}

func Create(ctx *context.ControllerContext) ([]syncer.Object, error) {
	registerContext := ToRegisterContext(ctx)

	// register controllers for resource synchronization
	syncers := []syncer.Object{}
	for k, v := range ResourceControllers {
		for _, controllerNew := range v {
			controllers := strings.Split(k, ",")
			for _, controller := range controllers {
				if ctx.Controllers[controller] {
					loghelper.Infof("Start %s sync controller", controller)
					ctrl, err := controllerNew(registerContext)
					if err != nil {
						return nil, errors.Wrapf(err, "register %s controller", controller)
					}

					syncers = append(syncers, ctrl)
					break
				}
			}
		}
	}

	return syncers, nil
}

func ExecuteInitializers(controllerCtx *context.ControllerContext, syncers []syncer.Object) error {
	registerContext := ToRegisterContext(controllerCtx)

	// execute in parallel because each one might be time-consuming
	errorGroup, ctx := errgroup.WithContext(controllerCtx.Context)
	registerContext.Context = ctx
	for _, s := range syncers {
		initializer, ok := s.(syncer.Initializer)
		if ok {
			errorGroup.Go(func() error {
				err := initializer.Init(registerContext)
				if err != nil {
					return errors.Wrapf(err, "ensure prerequisites for %s syncer", s.Name())
				}
				return nil
			})
		}
	}

	return errorGroup.Wait()
}

func RegisterIndices(ctx *context.ControllerContext, syncers []syncer.Object) error {
	registerContext := ToRegisterContext(ctx)
	for _, s := range syncers {
		indexRegisterer, ok := s.(syncer.IndicesRegisterer)
		if ok {
			err := indexRegisterer.RegisterIndices(registerContext)
			if err != nil {
				return errors.Wrapf(err, "register indices for %s syncer", s.Name())
			}
		}
	}

	return nil
}

func RegisterControllers(ctx *context.ControllerContext, syncers []syncer.Object) error {
	registerContext := ToRegisterContext(ctx)

	err := registerK8SDefaultEndpointController(ctx)
	if err != nil {
		return err
	}

	// register controller that maintains pod security standard check
	if ctx.Options.EnforcePodSecurityStandard != "" {
		err := registerPodSecurityController(ctx)
		if err != nil {
			return err
		}
	}

	// FixMe: Add description here
	if ctx.Options.EnforcePodSecurityStandard == "restricted" {
		err := registerDefaultSecurityContextController(ctx)
		if err != nil {
			return err
		}
	}

	// register controller that keeps CoreDNS NodeHosts config up to date
	err = registerCoreDNSController(ctx)
	if err != nil {
		return err
	}

	// register init manifests configmap watcher controller
	err = registerInitManifestsController(ctx)
	if err != nil {
		return err
	}

	// register controllers for resource synchronization
	for _, v := range syncers {
		// fake syncer?
		fakeSyncer, ok := v.(syncer.FakeSyncer)
		if ok {
			err = syncer.RegisterFakeSyncer(registerContext, fakeSyncer)
			if err != nil {
				return errors.Wrapf(err, "start %s syncer", v.Name())
			}
		} else {
			// real syncer?
			realSyncer, ok := v.(syncer.Syncer)
			if ok {
				err = syncer.RegisterSyncer(registerContext, realSyncer)
				if err != nil {
					return errors.Wrapf(err, "start %s syncer", v.Name())
				}
			} else {
				return fmt.Errorf("syncer %s does not implement fake syncer or syncer interface", v.Name())
			}
		}
	}

	return nil
}

func registerInitManifestsController(ctx *context.ControllerContext) error {
	controller := &manifests.InitManifestsConfigMapReconciler{
		LocalClient:    ctx.LocalManager.GetClient(),
		Log:            loghelper.New("initmanifests-controller"),
		VirtualManager: ctx.VirtualManager,
	}

	err := controller.SetupWithManager(ctx.LocalManager)
	if err != nil {
		return fmt.Errorf("unable to setup init manifests configmap controller: %v", err)
	}

	return nil
}

func registerCoreDNSController(ctx *context.ControllerContext) error {
	controller := &coredns.CoreDNSNodeHostsReconciler{
		Client: ctx.VirtualManager.GetClient(),
		Log:    loghelper.New("corednsnodehosts-controller"),
	}
	err := controller.SetupWithManager(ctx.VirtualManager)
	if err != nil {
		return fmt.Errorf("unable to setup CoreDNS NodeHosts controller: %v", err)
	}
	return nil
}

func registerPodSecurityController(ctx *context.ControllerContext) error {
	controller := &podsecurity.PodSecurityReconciler{
		Client:              ctx.VirtualManager.GetClient(),
		PodSecurityStandard: ctx.Options.EnforcePodSecurityStandard,
		Log:                 loghelper.New("podSecurity-controller"),
	}
	err := controller.SetupWithManager(ctx.VirtualManager)
	if err != nil {
		return fmt.Errorf("unable to setup pod security controller: %v", err)
	}
	return nil
}

func registerDefaultSecurityContextController(ctx *context.ControllerContext) error {
	controller := &defaultsecuritycontext.DefaultSecurityContextReconciler{
		Client: ctx.VirtualManager.GetClient(),
		Log:    loghelper.New("default-security-context-controller"),
	}
	err := controller.SetupWithManager(ctx.VirtualManager)
	if err != nil {
		return fmt.Errorf("unable to setup default security context controller: %v", err)
	}
	return nil
}

func registerK8SDefaultEndpointController(ctx *context.ControllerContext) error {
	controller := &k8sdefaultendpoint.K8SDefaultEndpointReconciler{
		LocalClient:         ctx.LocalManager.GetClient(),
		VirtualClient:       ctx.VirtualManager.GetClient(),
		ServiceName:         ctx.Options.ServiceName,
		ServiceNamespace:    ctx.CurrentNamespace,
		VirtualManagerCache: ctx.VirtualManager.GetCache(),
		Log:                 loghelper.New("kubernetes-default-endpoint-controller"),
	}
	err := controller.SetupWithManager(ctx.LocalManager)
	if err != nil {
		return fmt.Errorf("unable to setup pod security controller: %v", err)
	}
	return nil
}

func ToRegisterContext(ctx *context.ControllerContext) *synccontext.RegisterContext {
	return &synccontext.RegisterContext{
		Context: ctx.Context,

		Options:     ctx.Options,
		Controllers: ctx.Controllers,

		TargetNamespace:        ctx.Options.TargetNamespace,
		CurrentNamespace:       ctx.CurrentNamespace,
		CurrentNamespaceClient: ctx.CurrentNamespaceClient,

		VirtualManager:  ctx.VirtualManager,
		PhysicalManager: ctx.LocalManager,
	}
}

func newControllers(funcs ...func(*synccontext.RegisterContext) (syncer.Object, error)) []func(*synccontext.RegisterContext) (syncer.Object, error) {
	return append([]func(*synccontext.RegisterContext) (syncer.Object, error){}, funcs...)
}
