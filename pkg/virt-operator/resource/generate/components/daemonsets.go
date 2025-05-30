package components

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	virtv1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/pointer"
	"kubevirt.io/kubevirt/pkg/storage/reservation"
	"kubevirt.io/kubevirt/pkg/util"
	operatorutil "kubevirt.io/kubevirt/pkg/virt-operator/util"
)

const (
	VirtHandlerName = "virt-handler"
	kubeletPodsPath = util.KubeletRoot + "/pods"
	runtimesPath    = "/var/run/kubevirt-libvirt-runtimes"
	PrHelperName    = "pr-helper"
	prVolumeName    = "pr-helper-socket-vol"
	devDirVol       = "dev-dir"
	SidecarShimName = "sidecar-shim"
	etcMultipath    = "etc-multipath"
)

func RenderPrHelperContainer(image string, pullPolicy corev1.PullPolicy) corev1.Container {
	bidi := corev1.MountPropagationBidirectional
	return corev1.Container{
		Name:            PrHelperName,
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Command:         []string{"/entrypoint.sh"},
		Args: []string{
			"-k", reservation.GetPrHelperSocketPath(),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:             prVolumeName,
				MountPath:        reservation.GetPrHelperSocketDir(),
				MountPropagation: &bidi,
			},
			{
				Name:             devDirVol,
				MountPath:        "/dev",
				MountPropagation: pointer.P(corev1.MountPropagationHostToContainer),
			},
			{
				Name:             etcMultipath,
				MountPath:        "/etc/multipath",
				MountPropagation: &bidi,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  pointer.P(int64(util.RootUser)),
			Privileged: pointer.P(true),
		},
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}
}

func NewHandlerDaemonSet(namespace, repository, imagePrefix, version, launcherVersion, prHelperVersion, sidecarShimVersion, productName, productVersion, productComponent, image, launcherImage, prHelperImage, sidecarShimImage string, pullPolicy corev1.PullPolicy, imagePullSecrets []corev1.LocalObjectReference, migrationNetwork *string, verbosity string, extraEnv map[string]string, enablePrHelper bool) *appsv1.DaemonSet {

	deploymentName := VirtHandlerName
	imageName := fmt.Sprintf("%s%s", imagePrefix, deploymentName)
	env := operatorutil.NewEnvVarMap(extraEnv)
	podTemplateSpec := newPodTemplateSpec(deploymentName, imageName, repository, version, productName, productVersion, productComponent, image, pullPolicy, imagePullSecrets, nil, env)

	if launcherImage == "" {
		launcherImage = fmt.Sprintf("%s/%s%s%s", repository, imagePrefix, "virt-launcher", AddVersionSeparatorPrefix(launcherVersion))
	}

	if migrationNetwork != nil {
		if podTemplateSpec.ObjectMeta.Annotations == nil {
			podTemplateSpec.ObjectMeta.Annotations = make(map[string]string)
		}
		// Join the pod to the migration network and name the corresponding interface "migration0"
		podTemplateSpec.ObjectMeta.Annotations[networkv1.NetworkAttachmentAnnot] = *migrationNetwork + "@" + virtv1.MigrationInterfaceName
	}

	if podTemplateSpec.Annotations == nil {
		podTemplateSpec.Annotations = make(map[string]string)
	}
	podTemplateSpec.Annotations["openshift.io/required-scc"] = "kubevirt-handler"

	daemonset := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "DaemonSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      VirtHandlerName,
			Labels: map[string]string{
				virtv1.AppLabel: VirtHandlerName,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubevirt.io": VirtHandlerName,
				},
			},
			Template: *podTemplateSpec,
		},
	}

	if productVersion != "" {
		daemonset.ObjectMeta.Labels[virtv1.AppVersionLabel] = productVersion
	}

	if productName != "" {
		daemonset.ObjectMeta.Labels[virtv1.AppPartOfLabel] = productName
	}
	if productComponent != "" {
		daemonset.ObjectMeta.Labels[virtv1.AppComponentLabel] = productComponent
	}

	pod := &daemonset.Spec.Template.Spec
	pod.ServiceAccountName = HandlerServiceAccountName
	pod.HostPID = true

	// nodelabeller currently only support x86. The arch check will be done in node-labller.sh
	pod.InitContainers = []corev1.Container{
		{
			Command: []string{
				"/bin/sh",
				"-c",
			},
			Image: launcherImage,
			Name:  "virt-launcher",
			Args: []string{
				"node-labeller.sh",
			},
			SecurityContext: &corev1.SecurityContext{
				Privileged: pointer.P(true),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "node-labeller",
					MountPath: nodeLabellerVolumePath,
				},
			},
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

	// If there is any image pull secret added to the `virt-handler` deployment
	// it can mean that `virt-handler` is using private image. Therefore, we must
	// add `virt-launcher` container that will pre-pull and keep the (probably)
	// custom image of `virt-launcher`.
	// Note that we cannot make it an init container because the `virt-launcher`
	// image could be garbage collected by the kubelet.
	// Note that we cannot add `imagePullSecrets` to `virt-launcher` as this could
	// be a security risk - user could use this secret and abuse it.
	if len(imagePullSecrets) > 0 {
		pod.Containers = append(pod.Containers, corev1.Container{
			Name:            "virt-launcher-image-holder",
			Image:           launcherImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{"sleep infinity"},
			Resources: corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("20Mi"),
				},
			},
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		})
	}

	// give the handler grace period some padding
	// in order to ensure we have a chance to cleanly exit
	// before SIG_KILL
	podGracePeriod := int64(330)
	handlerGracePeriod := podGracePeriod - 15
	podTemplateSpec.Spec.TerminationGracePeriodSeconds = &podGracePeriod

	container := &pod.Containers[0]
	container.Command = []string{
		VirtHandlerName,
	}
	container.Args = []string{
		"--port",
		"8443",
		"--hostname-override",
		"$(NODE_NAME)",
		"--pod-ip-address",
		"$(MY_POD_IP)",
		"--max-metric-requests",
		"3",
		"--console-server-port",
		"8186",
		"--graceful-shutdown-seconds",
		fmt.Sprintf("%d", handlerGracePeriod),
		"-v",
		verbosity,
	}
	container.Ports = []corev1.ContainerPort{
		{
			Name:          "metrics",
			Protocol:      corev1.ProtocolTCP,
			ContainerPort: 8443,
		},
	}
	container.SecurityContext = &corev1.SecurityContext{
		Privileged: pointer.P(true),
		SELinuxOptions: &corev1.SELinuxOptions{
			Level: "s0",
		},
	}
	containerEnv := []corev1.EnvVar{
		{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		},
		{
			Name: "MY_POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}

	container.Env = append(container.Env, containerEnv...)

	container.LivenessProbe = &corev1.Probe{
		FailureThreshold: 3,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Scheme: corev1.URISchemeHTTPS,
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 8443,
				},
				Path: "/healthz",
			},
		},
		InitialDelaySeconds: 15,
		TimeoutSeconds:      10,
		PeriodSeconds:       45,
	}
	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Scheme: corev1.URISchemeHTTPS,
				Port: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 8443,
				},
				Path: "/healthz",
			},
		},
		InitialDelaySeconds: 15,
		TimeoutSeconds:      10,
		PeriodSeconds:       20,
	}

	type volume struct {
		name             string
		path             string
		mountPath        string
		mountPropagation *corev1.MountPropagationMode
	}
	attachCertificateSecret(pod, VirtHandlerCertSecretName, "/etc/virt-handler/clientcertificates")
	attachCertificateSecret(pod, VirtHandlerServerCertSecretName, "/etc/virt-handler/servercertificates")
	attachProfileVolume(pod)

	bidi := corev1.MountPropagationBidirectional
	// NOTE: the 'kubelet-pods' volume mount exists because that path holds unix socket files.
	// Socket files fail when their path is longer than 108 characters,
	//   so that shortened volume path is to allow domain socket connections.
	// It's ridiculous to have to account for that, but that's the situation we're in.
	volumes := []volume{
		{"libvirt-runtimes", runtimesPath, runtimesPath, nil},
		{"virt-share-dir", util.VirtShareDir, util.VirtShareDir, &bidi},
		{"virt-private-dir", util.VirtPrivateDir, util.VirtPrivateDir, nil},
		{"kubelet-pods", kubeletPodsPath, "/pods", nil},
		{"kubelet", util.KubeletRoot, util.KubeletRoot, &bidi},
		{"node-labeller", nodeLabellerVolumePath, nodeLabellerVolumePath, nil},
	}

	for _, volume := range volumes {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:             volume.name,
			MountPath:        volume.mountPath,
			MountPropagation: volume.mountPropagation,
		})
		pod.Volumes = append(pod.Volumes, corev1.Volume{
			Name: volume.name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: volume.path,
				},
			},
		})
	}

	// Use the downward API to access the network status annotations
	// TODO: This is not used anymore, but can't be removed because of https://github.com/kubevirt/kubevirt/issues/10632
	//   Since CR-based updates use the wrong install strategy, removing this volume and downgrading via CR will try to
	//   run the previous version of virt-handler without the volume, which will fail and CrashLoop.
	//   Please remove the volume once the above issue is fixed.
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "podinfo",
		MountPath: "/etc/podinfo",
	})
	pod.Volumes = append(pod.Volumes, corev1.Volume{
		Name: "podinfo",
		VolumeSource: corev1.VolumeSource{
			DownwardAPI: &corev1.DownwardAPIVolumeSource{
				Items: []corev1.DownwardAPIVolumeFile{
					{
						Path: "network-status",
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: `metadata.annotations['k8s.v1.cni.cncf.io/network-status']`,
						},
					},
				},
			},
		},
	})

	container.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10m"),
			corev1.ResourceMemory: resource.MustParse("325Mi"),
		},
	}
	if prHelperImage == "" {
		prHelperImage = fmt.Sprintf("%s/%s%s%s", repository, imagePrefix, PrHelperName, AddVersionSeparatorPrefix(prHelperVersion))
	}
	if sidecarShimImage == "" {
		sidecarShimImage = fmt.Sprintf("%s/%s%s%s", repository, imagePrefix, SidecarShimName, AddVersionSeparatorPrefix(sidecarShimVersion))
	}

	if enablePrHelper {
		directoryOrCreate := corev1.HostPathDirectoryOrCreate
		pod.Volumes = append(pod.Volumes, corev1.Volume{
			Name: prVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: reservation.GetPrHelperSocketDir(),
					Type: &directoryOrCreate,
				},
			}}, corev1.Volume{
			Name: devDirVol,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/dev",
				},
			}}, corev1.Volume{
			Name: etcMultipath,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/multipath",
					Type: pointer.P(corev1.HostPathDirectoryOrCreate),
				},
			}})
		pod.Containers = append(pod.Containers, RenderPrHelperContainer(prHelperImage, pullPolicy))
	}
	return daemonset

}
