package dmetrics_manager

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/tools/cache"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/downwardmetrics"
	virtioserial "kubevirt.io/kubevirt/pkg/downwardmetrics/virtio-serial"
	cmdclient "kubevirt.io/kubevirt/pkg/virt-handler/cmd-client"
	"kubevirt.io/kubevirt/pkg/virt-handler/isolation"
)

func NewDownwardMetricsManager(nodeName string, virtSharedDir string, vmiInformer cache.SharedIndexInformer) (*DownwardMetricsManager, error) {
	dmm := &DownwardMetricsManager{
		done:          false,
		nodeName:      nodeName,
		virtSharedDir: virtSharedDir,
		stopServer:    make(map[string]context.CancelFunc),
	}

	_, err := vmiInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			vmi, ok := obj.(*v1.VirtualMachineInstance)
			if !ok {
				log.Log.Errorf("v1.VirtualMachineInstance object failed conversion")
				return
			}
			dmm.stopDMServer(vmi)
		},
		UpdateFunc: func(_, obj interface{}) {
			vmi, ok := obj.(*v1.VirtualMachineInstance)
			if !ok {
				log.Log.Errorf("v1.VirtualMachineInstance object failed conversion")
				return
			}

			err := dmm.startDMServer(vmi)
			if err != nil {
				log.Log.Reason(err).Errorf("failed to start the DownwardMetrics virtio-serial stopServer")
			}
		},
	})

	if err != nil {
		return nil, err
	}
	return dmm, nil
}

// DownwardMetricsManager controls the lifetime of the DownwardMetrics servers.
// Each server is tied to the lifetime of the VMI and DownwardMetricsManager itself.
type DownwardMetricsManager struct {
	lock          sync.Mutex
	done          bool
	nodeName      string
	virtSharedDir string
	stopServer    map[string]context.CancelFunc
}

// Run blocks until stopCh is closed. When done, it stops all remaining
// running DownwardMetrics servers.
func (m *DownwardMetricsManager) Run(stopCh chan struct{}) {
	defer m.stop()
	<-stopCh
}

func (m *DownwardMetricsManager) stop() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.done = true

	// Stop all DownwardMetrics servers
	for domainName, stopServerFn := range m.stopServer {
		stopServerFn()
		delete(m.stopServer, domainName)
	}
}

func (m *DownwardMetricsManager) stopDMServer(vmi *v1.VirtualMachineInstance) {
	if vmi.Spec.Domain.Devices.DownwardMetrics == nil {
		return
	}

	domainName := fmt.Sprintf("%s/%s", vmi.GetNamespace(), vmi.GetName())

	m.lock.Lock()
	defer m.lock.Unlock()
	if m.done {
		return
	}
	m.stopServer[domainName]()
	delete(m.stopServer, domainName)
}

func (m *DownwardMetricsManager) startDMServer(vmi *v1.VirtualMachineInstance) error {
	if vmi.Spec.Domain.Devices.DownwardMetrics == nil {
		return nil
	}

	if !vmi.IsRunning() {
		return nil
	}

	domainName := fmt.Sprintf("%s/%s", vmi.GetNamespace(), vmi.GetName())

	m.lock.Lock()
	defer m.lock.Unlock()
	if m.done {
		return nil
	}

	if _, alreadyStarted := m.stopServer[domainName]; alreadyStarted {
		return nil
	}

	launcherSocketPath, err := cmdclient.FindSocketOnHost(vmi)
	if err != nil {
		return fmt.Errorf("failed to get the launcher socket for VMI [%s], error: %v", vmi.GetName(), err)
	}

	socketDetector := isolation.NewSocketBasedIsolationDetector(m.virtSharedDir)
	res, err := socketDetector.Detect(vmi)
	if err != nil {
		return fmt.Errorf("failed to detect root directory of the vmi pod for VMI [%s], error: %v", vmi.GetName(), err)
	}

	channelPath := downwardmetrics.ChannelSocketPathOnHost(res.Pid())
	ctx, cancelCtx := context.WithCancel(context.Background())
	err = virtioserial.RunDownwardMetricsVirtioServer(ctx, m.nodeName, channelPath, launcherSocketPath)
	if err != nil {
		cancelCtx()
		return fmt.Errorf("failed to start the DownwardMetrics stopServer for VMI [%s], error: %v", vmi.GetName(), err)
	}
	m.stopServer[domainName] = cancelCtx

	return nil
}
