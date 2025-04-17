package source

import (
	"fmt"

	kubevirt "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
)

const (
	NetworkInterfaceModelE1000   = "e1000"
	NetworkInterfaceModelE1000e  = "e1000e"
	NetworkInterfaceModelNe2kPci = "ne2k_pci"
	NetworkInterfaceModelPcnet   = "pcnet"
	NetworkInterfaceModelRtl8139 = "rtl8139"
	NetworkInterfaceModelVirtio  = "virtio"
)

type NetworkInfo struct {
	NetworkName   string
	MAC           string
	MappedNetwork string
	// This can be one of: e1000, e1000e, ne2k_pci, pcnet, rtl8139, virtio.
	// See https://kubevirt.io/user-guide/network/interfaces_and_networks/#interfaces
	Model string
}

func MapNetworkCards(networkInfos []NetworkInfo, networkMappings []migration.NetworkMapping) []NetworkInfo {
	var result []NetworkInfo
	for _, ni := range networkInfos {
		for _, nm := range networkMappings {
			if nm.SourceNetwork == ni.NetworkName {
				ni.MappedNetwork = nm.DestinationNetwork

				// Override the auto-detected interface model if it is
				// customized by the user via the `NetworkMapping` in the
				// `VirtualMachineImportSpec`.
				if nm.InterfaceModel != "" {
					ni.Model = nm.InterfaceModel
				}
				// Set a default interface model if none was auto-detected
				// nor customized by the user.
				if ni.Model == "" {
					ni.Model = NetworkInterfaceModelVirtio
				}

				result = append(result, ni)
			}
		}
	}
	return result
}

func GenerateNetworkInterfaceConfig(networkInfos []NetworkInfo, managementNetworkInterfaceModel string) ([]kubevirt.Network, []kubevirt.Interface) {
	networks := make([]kubevirt.Network, 0, len(networkInfos))
	interfaces := make([]kubevirt.Interface, 0, len(networkInfos))

	for i, ni := range networkInfos {
		networks = append(networks, kubevirt.Network{
			NetworkSource: kubevirt.NetworkSource{
				Multus: &kubevirt.MultusNetwork{
					NetworkName: ni.MappedNetwork,
				},
			},
			Name: fmt.Sprintf("migrated-%d", i),
		})

		interfaces = append(interfaces, kubevirt.Interface{
			Name:       fmt.Sprintf("migrated-%d", i),
			MacAddress: ni.MAC,
			Model:      ni.Model,
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Bridge: &kubevirt.InterfaceBridge{},
			},
		})
	}

	// If there is no network, attach to Pod network. Essential for VM to
	// be booted up.
	if len(networks) == 0 {
		networks = append(networks, kubevirt.Network{
			Name: "pod-network",
			NetworkSource: kubevirt.NetworkSource{
				Pod: &kubevirt.PodNetwork{},
			},
		})
		interfaces = append(interfaces, kubevirt.Interface{
			Name:  "pod-network",
			Model: managementNetworkInterfaceModel,
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Masquerade: &kubevirt.InterfaceMasquerade{},
			},
		})
	}

	return networks, interfaces
}
