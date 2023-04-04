package converter

import (
	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/config"

	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
)

func convertDownwardMetricsChannel(downwardMetrics *v1.DownwardMetrics) []api.Channel {
	var domainDownwardMetricChannel []api.Channel

	if downwardMetrics == nil {
		return domainDownwardMetricChannel
	}

	domainDownwardMetricChannel = append(domainDownwardMetricChannel,
		api.Channel{
			Type: "unix",
			Source: &api.ChannelSource{
				Mode: "bind",
				//FIXME: change this for the socket path
				Path: config.DownwardMetricDisk,
			},
			Target: &api.ChannelTarget{
				Type: v1.VirtIO,
				Name: "org.github.vhostmd.1",
			},
			Address: &api.ChannelAddress{
				Type:       v1.VirtIOSerial,
				Controller: 0,
				Bus:        0,
				Port:       1,
			},
		})

	return domainDownwardMetricChannel
}
