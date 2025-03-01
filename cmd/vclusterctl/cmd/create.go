package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/loft-sh/vcluster/cmd/vclusterctl/cmd/app/localkubernetes"
	"github.com/loft-sh/vcluster/cmd/vclusterctl/cmd/find"
	"github.com/loft-sh/vcluster/cmd/vclusterctl/log/survey"
	"github.com/loft-sh/vcluster/cmd/vclusterctl/log/terminal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/loft-sh/vcluster/cmd/vclusterctl/cmd/app/create"
	"github.com/loft-sh/vcluster/pkg/helm/values"
	"github.com/loft-sh/vcluster/pkg/upgrade"
	"github.com/loft-sh/vcluster/pkg/util"
	"github.com/loft-sh/vcluster/pkg/util/servicecidr"
	"golang.org/x/mod/semver"

	"github.com/loft-sh/vcluster/cmd/vclusterctl/flags"
	"github.com/loft-sh/vcluster/cmd/vclusterctl/log"
	"github.com/loft-sh/vcluster/pkg/helm"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	AllowedDistros              = []string{"k3s", "k0s", "k8s", "eks"}
	CreatedByVClusterAnnotation = "vcluster.loft.sh/created"
)

var tty = terminal.SetupTTY(os.Stdin, os.Stdout)
var isTerminalIn = tty.IsTerminalIn()

const LoftChartRepo = "https://charts.loft.sh"

// CreateCmd holds the login cmd flags
type CreateCmd struct {
	*flags.GlobalFlags
	create.CreateOptions

	log log.Logger

	localCluster     bool
	kubeClientConfig clientcmd.ClientConfig
	kubeClient       *kubernetes.Clientset
	rawConfig        clientcmdapi.Config
}

// NewCreateCmd creates a new command
func NewCreateCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &CreateCmd{
		GlobalFlags: globalFlags,
		log:         log.GetInstance(),
	}

	cobraCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new virtual cluster",
		Long: `
#######################################################
################### vcluster create ###################
#######################################################
Creates a new virtual cluster

Example:
vcluster create test --namespace test
#######################################################
	`,
		Args: cobra.ExactArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			// Check for newer version
			upgrade.PrintNewerVersionWarning()
			validateDeprecated(&cmd.CreateOptions, cmd.log)
			return cmd.Run(args)
		},
	}

	cobraCmd.Flags().StringVar(&cmd.KubeConfigContextName, "kube-config-context-name", "", "If set, will override the context name of the generated virtual cluster kube config with this name")
	cobraCmd.Flags().StringVar(&cmd.ChartVersion, "chart-version", upgrade.GetVersion(), "The virtual cluster chart version to use (e.g. v0.9.1)")
	cobraCmd.Flags().StringVar(&cmd.ChartName, "chart-name", "vcluster", "The virtual cluster chart name to use")
	cobraCmd.Flags().StringVar(&cmd.ChartRepo, "chart-repo", LoftChartRepo, "The virtual cluster chart repo to use")
	cobraCmd.Flags().StringVar(&cmd.LocalChartDir, "local-chart-dir", "", "The virtual cluster local chart dir to use")
	cobraCmd.Flags().StringVar(&cmd.K3SImage, "k3s-image", "", "DEPRECATED: use --extra-values instead")
	cobraCmd.Flags().StringVar(&cmd.Distro, "distro", "k3s", fmt.Sprintf("Kubernetes distro to use for the virtual cluster. Allowed distros: %s", strings.Join(AllowedDistros, ", ")))
	cobraCmd.Flags().StringVar(&cmd.ReleaseValues, "release-values", "", "DEPRECATED: use --extra-values instead")
	cobraCmd.Flags().StringVar(&cmd.KubernetesVersion, "kubernetes-version", "", "The kubernetes version to use (e.g. v1.20). Patch versions are not supported")
	cobraCmd.Flags().StringSliceVarP(&cmd.ExtraValues, "extra-values", "f", []string{}, "Path where to load extra helm values from")
	cobraCmd.Flags().BoolVar(&cmd.CreateNamespace, "create-namespace", true, "If true the namespace will be created if it does not exist")
	cobraCmd.Flags().BoolVar(&cmd.DisableIngressSync, "disable-ingress-sync", false, "If true the virtual cluster will not sync any ingresses")
	cobraCmd.Flags().BoolVar(&cmd.UpdateCurrent, "update-current", true, "If true updates the current kube config")
	cobraCmd.Flags().BoolVar(&cmd.CreateClusterRole, "create-cluster-role", false, "DEPRECATED: cluster role is now automatically created if it is required by one of the resource syncers that are enabled by the .sync.RESOURCE.enabled=true helm value, which is set in a file that is passed via --extra-values argument.")
	cobraCmd.Flags().BoolVar(&cmd.Expose, "expose", false, "If true will create a load balancer service to expose the vcluster endpoint")
	cobraCmd.Flags().BoolVar(&cmd.ExposeLocal, "expose-local", true, "If true and a local Kubernetes distro is detected, will deploy vcluster with a NodePort service")

	cobraCmd.Flags().BoolVar(&cmd.Connect, "connect", true, "If true will run vcluster connect directly after the vcluster was created")
	cobraCmd.Flags().BoolVar(&cmd.Upgrade, "upgrade", false, "If true will try to upgrade the vcluster instead of failing if it already exists")
	cobraCmd.Flags().BoolVar(&cmd.Isolate, "isolate", false, "If true vcluster and its workloads will run in an isolated environment")
	return cobraCmd
}

func validateDeprecated(createOptions *create.CreateOptions, log log.Logger) {
	if createOptions.ReleaseValues != "" {
		log.Warn("Flag --release-values is deprecated, please use --extra-values instead. This flag will be removed in future!")
	}
	if createOptions.K3SImage != "" {
		log.Warn("Flag --k3s-image is deprecated, please use --extra-values instead. This flag will be removed in future!")
	}
	if createOptions.CreateClusterRole {
		log.Warn("Flag --create-cluster-role is deprecated. Cluster role is now automatically created if it is required by one of the resource syncers that are enabled by the .sync.RESOURCE.enabled=true helm value, which is set in a file that is passed via --extra-values (or -f) argument.")
	}
}

// Run executes the functionality
func (cmd *CreateCmd) Run(args []string) error {
	helmBinaryPath, err := GetHelmBinaryPath(cmd.log)
	if err != nil {
		return err
	}

	_, err = exec.Command(helmBinaryPath, "version").CombinedOutput()
	if err != nil {
		return err
	}

	err = cmd.prepare(args[0])
	if err != nil {
		return err
	}

	// find out kubernetes version
	kubernetesVersion, err := cmd.getKubernetesVersion()
	if err != nil {
		return err
	}

	vclusterExistsInNamespace, err := cmd.vclusterExistsInNamespace()
	if err != nil {
		cmd.log.Errorf("error while listing clusters to check if the vcluster is already present in the namespace : %s", err)
		return err
	}
	if !cmd.Upgrade && vclusterExistsInNamespace {
		cmd.log.Errorf("another vcluster is already present in %s namespace, please try another namespace.", cmd.Namespace)
		return nil
	}

	// load the default values
	chartOptions, err := cmd.ToChartOptions(kubernetesVersion)
	if err != nil {
		return err
	}
	chartValues, err := values.GetDefaultReleaseValues(chartOptions, cmd.log)
	if err != nil {
		return err
	}

	var newExtraValues []string
	for _, value := range cmd.ExtraValues {
		decodedString, err := getBase64DecodedString(value)
		// ignore decoding errors and treat it as non-base64 string
		if err != nil {
			newExtraValues = append(newExtraValues, value)
			continue
		}

		// write a temporary values file
		tempFile, err := os.CreateTemp("", "")
		tempValuesFile := tempFile.Name()
		if err != nil {
			return errors.Wrap(err, "create temp values file")
		}
		defer func(name string) {
			_ = os.Remove(name)
		}(tempValuesFile)

		_, err = tempFile.Write([]byte(decodedString))
		if err != nil {
			return errors.Wrap(err, "write values to temp values file")
		}

		err = tempFile.Close()
		if err != nil {
			return errors.Wrap(err, "close temp values file")
		}
		// setting new file to extraValues slice to process it further.
		newExtraValues = append(newExtraValues, tempValuesFile)
	}

	// resetting this as the base64 encoded strings should be removed and only valid file names should be kept.
	cmd.ExtraValues = newExtraValues

	if cmd.ReleaseValues != "" {
		cmd.ExtraValues = append(cmd.ExtraValues, cmd.ReleaseValues)
	}

	// check if vcluster already exists
	if !cmd.Upgrade {
		release, err := helm.NewSecrets(cmd.kubeClient).Get(context.Background(), args[0], cmd.Namespace)
		if err != nil && !kerrors.IsNotFound(err) {
			return errors.Wrap(err, "get helm releases")
		} else if release != nil && release.Chart != nil && release.Chart.Metadata != nil && (release.Chart.Metadata.Name == "vcluster" || release.Chart.Metadata.Name == "vcluster-k0s" || release.Chart.Metadata.Name == "vcluster-k8s") && release.Secret != nil && release.Secret.Labels != nil && release.Secret.Labels["status"] == "deployed" {
			if cmd.Connect {
				connectCmd := &ConnectCmd{
					GlobalFlags:           cmd.GlobalFlags,
					UpdateCurrent:         cmd.UpdateCurrent,
					KubeConfigContextName: cmd.KubeConfigContextName,
					KubeConfig:            "./kubeconfig.yaml",
					Log:                   cmd.log,
				}

				return connectCmd.Connect(args[0], nil)
			}

			return fmt.Errorf("vcluster %s already exists in namespace %s\n- Use `vcluster create %s -n %s --upgrade` to upgrade the vcluster\n- Use `vcluster connect %s -n %s` to access the vcluster", args[0], cmd.Namespace, args[0], cmd.Namespace, args[0], cmd.Namespace)
		}
	}

	// we have to upgrade / install the chart
	err = cmd.deployChart(args[0], chartValues, helmBinaryPath)
	if err != nil {
		return err
	}

	// check if we should connect to the vcluster
	if cmd.Connect {
		cmd.log.Donef("Successfully created virtual cluster %s in namespace %s", args[0], cmd.Namespace)
		connectCmd := &ConnectCmd{
			GlobalFlags:           cmd.GlobalFlags,
			UpdateCurrent:         cmd.UpdateCurrent,
			KubeConfigContextName: cmd.KubeConfigContextName,
			KubeConfig:            "./kubeconfig.yaml",
			Log:                   cmd.log,
		}

		return connectCmd.Connect(args[0], nil)
	} else {
		if cmd.localCluster {
			cmd.log.Donef("Successfully created virtual cluster %s in namespace %s. \n- Use 'vcluster connect %s --namespace %s' to access the virtual cluster", args[0], cmd.Namespace, args[0], cmd.Namespace)
		} else {
			cmd.log.Donef("Successfully created virtual cluster %s in namespace %s. \n- Use 'vcluster connect %s --namespace %s' to access the virtual cluster\n- Use `vcluster connect %s --namespace %s -- kubectl get ns` to run a command directly within the vcluster", args[0], cmd.Namespace, args[0], cmd.Namespace, args[0], cmd.Namespace)
		}
	}

	return nil
}

func getBase64DecodedString(values string) (string, error) {
	strDecoded, err := base64.StdEncoding.DecodeString(values)
	if err != nil {
		return "", err
	}
	return string(strDecoded), nil
}

func (cmd *CreateCmd) deployChart(vClusterName, chartValues, helmExecutablePath string) error {
	// check if there is a vcluster directory already
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current work directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, cmd.ChartName)); err == nil {
		return fmt.Errorf("aborting vcluster creation. Current working directory contains a file or a directory with the name equal to the vcluster chart name - \"%s\". Please execute vcluster create command from a directory that doesn't contain a file or directory named \"%s\"", cmd.ChartName, cmd.ChartName)
	}

	// rewrite chart location, this is an optimization to avoid
	// downloading the whole index.yaml and parsing it
	if cmd.LocalChartDir == "" && cmd.ChartVersion != "" && cmd.ChartRepo == LoftChartRepo {
		if cmd.ChartVersion[0] == 'v' {
			cmd.ChartVersion = cmd.ChartVersion[1:]
		}

		cmd.LocalChartDir = LoftChartRepo + "/charts/" + cmd.ChartName + "-" + cmd.ChartVersion + ".tgz"
		cmd.ChartVersion = ""
		cmd.ChartRepo = ""
	}

	if cmd.Upgrade {
		cmd.log.Infof("Upgrade vcluster %s...", vClusterName)
	} else {
		cmd.log.Infof("Create vcluster %s...", vClusterName)
	}

	// we have to upgrade / install the chart
	ctx := context.Background()
	err = helm.NewClient(&cmd.rawConfig, cmd.log, helmExecutablePath).Upgrade(ctx, vClusterName, cmd.Namespace, helm.UpgradeOptions{
		Chart:       cmd.ChartName,
		Path:        cmd.LocalChartDir,
		Repo:        cmd.ChartRepo,
		Version:     cmd.ChartVersion,
		Values:      chartValues,
		ValuesFiles: cmd.ExtraValues,
	})
	if err != nil {
		return err
	}

	return nil
}

func (cmd *CreateCmd) ToChartOptions(kubernetesVersion *version.Info) (*helm.ChartOptions, error) {
	if !util.Contains(cmd.Distro, AllowedDistros) {
		return nil, fmt.Errorf("unsupported distro %s, please select one of: %s", cmd.Distro, strings.Join(AllowedDistros, ", "))
	}

	if cmd.ChartName == "vcluster" && cmd.Distro != "k3s" {
		cmd.ChartName += "-" + cmd.Distro
	}

	// check if we should create with node port
	clusterType := localkubernetes.DetectClusterType(&cmd.rawConfig)
	if cmd.ExposeLocal && clusterType.LocalKubernetes() {
		cmd.log.Infof("Detected local kubernetes cluster %s. Will deploy vcluster with a NodePort & sync real nodes", clusterType)
		cmd.localCluster = true
	}

	return &helm.ChartOptions{
		ChartName:          cmd.ChartName,
		ChartRepo:          cmd.ChartRepo,
		ChartVersion:       cmd.ChartVersion,
		CIDR:               cmd.CIDR,
		CreateClusterRole:  cmd.CreateClusterRole,
		DisableIngressSync: cmd.DisableIngressSync,
		Expose:             cmd.Expose,
		SyncNodes:          cmd.localCluster,
		NodePort:           cmd.localCluster,
		K3SImage:           cmd.K3SImage,
		Isolate:            cmd.Isolate,
		KubernetesVersion:  kubernetesVersion,
	}, nil
}

func (cmd *CreateCmd) prepare(vClusterName string) error {
	// first load the kube config
	kubeClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{
		CurrentContext: cmd.Context,
	})

	// load the raw config
	rawConfig, err := kubeClientConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}
	if cmd.Context != "" {
		rawConfig.CurrentContext = cmd.Context
	}

	// check if vcluster in vcluster
	_, _, previousContext := find.VClusterFromContext(rawConfig.CurrentContext)
	if previousContext != "" {
		if isTerminalIn {
			switchBackOption := "No, switch back to context " + previousContext
			out, err := cmd.log.Question(&survey.QuestionOptions{
				Question:     "You are creating a vcluster inside another vcluster, is this desired?",
				DefaultValue: switchBackOption,
				Options:      []string{switchBackOption, "Yes"},
			})
			if err != nil {
				return err
			}

			if out == switchBackOption {
				cmd.Context = previousContext
				kubeClientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{
					CurrentContext: cmd.Context,
				})
				rawConfig, err = kubeClientConfig.RawConfig()
				if err != nil {
					return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
				}
				err = switchContext(&rawConfig, cmd.Context)
				if err != nil {
					return errors.Wrap(err, "switch context")
				}
			}
		} else {
			cmd.log.Warnf("You are creating a vcluster inside another vcluster, is this desired?")
		}
	}

	// load the rest config
	kubeConfig, err := kubeClientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%v), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	cmd.kubeClient = client
	cmd.kubeClientConfig = kubeClientConfig
	cmd.rawConfig = rawConfig

	// ensure namespace
	err = cmd.ensureNamespace(vClusterName)
	if err != nil {
		return err
	}

	// get service cidr
	if cmd.CIDR == "" {
		cmd.CIDR = servicecidr.GetServiceCIDR(cmd.kubeClient, cmd.Namespace)
	}

	return nil
}

func (cmd *CreateCmd) ensureNamespace(vClusterName string) error {
	var err error
	if cmd.Namespace == "" {
		cmd.Namespace, _, err = cmd.kubeClientConfig.Namespace()
		if err != nil {
			return err
		} else if cmd.Namespace == "" || cmd.Namespace == "default" {
			cmd.Namespace = "vcluster-" + vClusterName
			cmd.log.Debugf("Will use namespace %s to create the vcluster...", cmd.Namespace)
		}
	}

	// make sure namespace exists
	namespace, err := cmd.kubeClient.CoreV1().Namespaces().Get(context.Background(), cmd.Namespace, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return cmd.createNamespace()
		} else if !kerrors.IsForbidden(err) {
			return err
		}
	} else if namespace.DeletionTimestamp != nil {
		cmd.log.Infof("Waiting until namespace is terminated...")
		err := wait.Poll(time.Second, time.Minute*2, func() (bool, error) {
			namespace, err := cmd.kubeClient.CoreV1().Namespaces().Get(context.Background(), cmd.Namespace, metav1.GetOptions{})
			if err != nil {
				if kerrors.IsNotFound(err) {
					return true, nil
				}

				return false, err
			}

			return namespace.DeletionTimestamp == nil, nil
		})
		if err != nil {
			return err
		}

		// create namespace
		return cmd.createNamespace()
	}

	return nil
}

func (cmd *CreateCmd) createNamespace() error {
	// try to create the namespace
	cmd.log.Infof("Creating namespace %s", cmd.Namespace)
	_, err := cmd.kubeClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cmd.Namespace,
			Annotations: map[string]string{
				CreatedByVClusterAnnotation: "true",
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create namespace")
	}
	return nil
}

func (cmd *CreateCmd) getKubernetesVersion() (*version.Info, error) {
	var (
		kubernetesVersion *version.Info
		err               error
	)
	if cmd.KubernetesVersion != "" {
		if cmd.KubernetesVersion[0] != 'v' {
			cmd.KubernetesVersion = "v" + cmd.KubernetesVersion
		}

		if !semver.IsValid(cmd.KubernetesVersion) {
			return nil, fmt.Errorf("please use valid semantic versioning format, e.g. vX.X")
		}

		majorMinorVer := semver.MajorMinor(cmd.KubernetesVersion)

		if splittedVersion := strings.Split(cmd.KubernetesVersion, "."); len(splittedVersion) > 2 {
			cmd.log.Warnf("currently we only support major.minor version (%s) and not the patch version (%s)", majorMinorVer, cmd.KubernetesVersion)
		}

		kubernetesVersion, err = values.ParseKubernetesVersionInfo(majorMinorVer)
		if err != nil {
			return nil, err
		}
	}

	if kubernetesVersion == nil {
		kubernetesVersion, err = cmd.kubeClient.DiscoveryClient.ServerVersion()
		if err != nil {
			return nil, err
		}
	}

	return kubernetesVersion, nil
}

func (cmd *CreateCmd) vclusterExistsInNamespace() (bool, error) {
	vClusters, err := find.ListVClusters(cmd.Context, "", cmd.Namespace)
	if err != nil {
		return false, err
	}
	for _, cluster := range vClusters {
		if cluster.Namespace == cmd.Namespace {
			return true, nil
		}
	}
	return false, nil
}
