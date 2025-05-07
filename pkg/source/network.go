package source

import (
	"fmt"

	kubevirt "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
)

type NetworkInfo struct {
	NetworkName   string
	MAC           string
	MappedNetwork string
	Model         string
}

func MapNetworks(networkInfos []NetworkInfo, networkMappings []migration.NetworkMapping) []NetworkInfo {
	result := make([]NetworkInfo, 0)

	for _, ni := range networkInfos {
		for _, nm := range networkMappings {
			if nm.SourceNetwork == ni.NetworkName {
				ni.MappedNetwork = nm.DestinationNetwork

				// Override the auto-detected interface model if it is
				// customized by the user via the `NetworkMapping`.
				if nm.NetworkInterfaceModel != nil {
					ni.Model = nm.GetNetworkInterfaceModel()
				}

				result = append(result, ni)
			}
		}
	}

	return result
}

func GenerateNetworkInterfaceConfigs(networkInfos []NetworkInfo, defaultNetworkInterfaceModel string) ([]kubevirt.Network, []kubevirt.Interface) {
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
			Model: defaultNetworkInterfaceModel,
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Masquerade: &kubevirt.InterfaceMasquerade{},
			},
		})
	}

	return networks, interfaces
}
