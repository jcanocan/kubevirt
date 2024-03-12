package virtiofs

import (
	"fmt"
	"path/filepath"

	virtconfig "kubevirt.io/kubevirt/pkg/virt-config"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/util"
)

// This is empty dir
var VirtioFSContainers = "virtiofs-containers"
var VirtioFSContainersMountBaseDir = filepath.Join(util.VirtShareDir, VirtioFSContainers)

func VirtioFSSocketPath(volumeName string) string {
	socketName := fmt.Sprintf("%s.sock", volumeName)
	return filepath.Join(VirtioFSContainersMountBaseDir, socketName)
}

// CanRunWithPrivileges returns true if the virtiofs container of the volume
// can run as user root
func CanRunWithPrivileges(config *virtconfig.ClusterConfig, volume *v1.Volume) bool {
	// config volumes does not require a privileged container
	return config.RootEnabled() && volume.ConfigMap == nil && volume.Secret == nil &&
		volume.ServiceAccount == nil && volume.DownwardAPI == nil
}
