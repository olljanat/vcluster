package e2erootlessmode

import (
	"strings"
	"time"

	"github.com/loft-sh/vcluster/pkg/util/podhelper"
	"github.com/loft-sh/vcluster/test/framework"
	"github.com/onsi/ginkgo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = ginkgo.Describe("Rootless mode", func() {
	f := framework.DefaultFramework
	ginkgo.It("run vcluster in rootless mode", func() {
		pods, err := f.HostClient.CoreV1().Pods(f.VclusterNamespace).List(f.Context, metav1.ListOptions{
			LabelSelector: "app=vcluster",
		})
		framework.ExpectNoError(err)
		vclusterPod := pods.Items[0].Name
		cmd := []string{
			"/bin/sh",
			"-c",
			"id -u",
		}
		stdout, stderr, err := podhelper.ExecBuffered(f.HostConfig,
			f.VclusterNamespace,
			vclusterPod,
			"syncer",
			cmd,
			nil)
		framework.ExpectNoError(err)
		framework.ExpectEqual(0, len(stderr))
		framework.ExpectEqual("12345", strings.TrimSuffix(string(stdout), "\n"))

		ginkgo.By("Create rootless workload in vcluster and verify if it's running")
		pod := &corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "nginx",
			},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{},
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							corev1.VolumeMount{
								Name:      "nginx-cache",
								MountPath: "/var/cache/nginx/",
							},
							corev1.VolumeMount{
								Name:      "nginx-run",
								MountPath: "/var/run/",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					corev1.Volume{
						Name: "nginx-cache",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: &resource.Quantity{
									Format: "100Mi",
								},
							},
						},
					},
					corev1.Volume{
						Name: "nginx-run",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: &resource.Quantity{
									Format: "1Mi",
								},
							},
						},
					},
				},
			},
		}
		*pod.Spec.SecurityContext.RunAsNonRoot = true
		*pod.Spec.SecurityContext.RunAsUser = int64(65534)

		_, err = f.VclusterClient.CoreV1().Pods("default").Create(f.Context, pod, metav1.CreateOptions{})
		framework.ExpectNoError(err)

		err = wait.Poll(time.Second, time.Minute*2, func() (bool, error) {
			p, _ := f.VclusterClient.CoreV1().Pods("default").Get(f.Context, "nginx", metav1.GetOptions{})
			if p.Status.Phase == corev1.PodRunning {
				return true, nil
			}
			return false, nil
		})
		framework.ExpectNoError(err)

		p, err := f.HostClient.CoreV1().Pods(f.VclusterNamespace).List(f.Context, metav1.ListOptions{
			LabelSelector: "vcluster.loft.sh/namespac=default",
		})
		framework.ExpectNoError(err)
		framework.ExpectEqual(true, len(p.Items) > 0)
	})
})
