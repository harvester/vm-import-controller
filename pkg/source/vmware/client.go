package vmware

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/nfc"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirt "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/qemu"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
	"github.com/harvester/vm-import-controller/pkg/util"
)

type Client struct {
	ctx context.Context
	*govmomi.Client
	tmpCerts       string
	dc             string
	networkMapping map[string]string
}

func NewClient(ctx context.Context, endpoint string, dc string, secret *corev1.Secret) (*Client, error) {
	var insecure bool
	username, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("no key username found in secret %s", secret.Name)
	}
	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("no key password found in the secret %s", secret.Name)
	}

	caCert, ok := secret.Data["caCert"]
	if !ok {
		insecure = true
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error parsing endpoint url: %v", err)
	}

	sc := soap.NewClient(endpointURL, insecure)

	vmwareClient := &Client{}
	if !insecure {
		tmpFile, err := os.CreateTemp("/tmp", "vmware-ca-")
		if err != nil {
			return nil, fmt.Errorf("error creating tmp file for vmware ca certs: %v", err)
		}
		_, err = tmpFile.Write(caCert)
		if err != nil {
			return nil, fmt.Errorf("error writing ca cert to tmp file %s: %v", tmpFile.Name(), err)
		}
		if err = sc.SetRootCAs(tmpFile.Name()); err != nil {
			return nil, err
		}
		vmwareClient.tmpCerts = tmpFile.Name()
	}

	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, fmt.Errorf("error creating vim client: %v", err)
	}
	c := &govmomi.Client{
		Client:         vc,
		SessionManager: session.NewManager(vc),
	}

	err = c.Login(ctx, url.UserPassword(string(username), string(password)))
	if err != nil {
		return nil, fmt.Errorf("error during login :%v", err)
	}

	vmwareClient.ctx = ctx
	vmwareClient.Client = c
	vmwareClient.dc = dc
	networkMap, err := GenerateNetworkMapByRef(ctx, c.Client)
	if err != nil {
		return nil, fmt.Errorf("error generating network map during client initialisation: %w", err)
	}
	vmwareClient.networkMapping = networkMap
	return vmwareClient, nil

}

func (c *Client) Close() error {
	c.Client.CloseIdleConnections()
	err := c.Client.Logout(c.ctx)
	if err != nil {
		return err
	}
	return os.Remove(c.tmpCerts)
}

// Verify checks is a verification check for migration provider to ensure that the config is valid
// it is used to set the condition Ready on the migration provider.
// for vmware client we verify if the DC exists
func (c *Client) Verify() error {
	f := find.NewFinder(c.Client.Client, true)
	dc := c.dc
	if !strings.HasPrefix(c.dc, "/") {
		dc = fmt.Sprintf("/%s", c.dc)
	}

	dcObj, err := f.Datacenter(c.ctx, dc)
	if err != nil {
		return err
	}

	logrus.Infof("found dc: %v", dcObj)
	return nil
}

func (c *Client) PreFlightChecks(vm *migration.VirtualMachineImport) (err error) {
	// Check the source network mappings.
	if vm.Spec.SkipPreflightChecks {
		logrus.WithFields(logrus.Fields{
			"name":      vm.Name,
			"namespace": vm.Namespace,
		}).Info("skipping preflight checks")
		return nil
	}

	networkMap, err := GenerateNetworkMapByName(c.ctx, c.Client.Client)
	if err != nil {
		return fmt.Errorf("error generating network map: %v", err)
	}
	for _, nm := range vm.Spec.Mapping {
		logrus.WithFields(logrus.Fields{
			"name":          vm.Name,
			"namespace":     vm.Namespace,
			"sourceNetwork": nm.SourceNetwork,
		}).Info("Checking the source network as part of the preflight checks")

		elements := strings.Split(nm.SourceNetwork, "/")
		if _, ok := networkMap[elements[len(elements)-1]]; ok {
			return nil
		}
	}

	return nil
}

func (c *Client) ExportVirtualMachine(vm *migration.VirtualMachineImport) (err error) {
	var (
		tmpPath string
		vmObj   *object.VirtualMachine
		lease   *nfc.Lease
		info    *nfc.LeaseInfo
	)
	tmpPath, err = os.MkdirTemp("/tmp", fmt.Sprintf("%s-%s-", vm.Name, vm.Namespace))

	if err != nil {
		return fmt.Errorf("error creating tmp dir in ExportVirtualMachine: %v", err)
	}

	vmObj, err = c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return fmt.Errorf("error finding vm in ExportVirtualMachine: %v", err)
	}

	lease, err = vmObj.Export(c.ctx)
	if err != nil {
		return fmt.Errorf("error generate export lease in ExportVirtualMachine: %v", err)
	}

	info, err = lease.Wait(c.ctx, nil)
	if err != nil {
		return err
	}

	u := lease.StartUpdater(c.ctx, info)
	defer os.RemoveAll(tmpPath)

	for _, i := range info.Items {
		// ignore iso and nvram disks
		if strings.HasSuffix(i.Path, ".vmdk") {
			if !strings.HasPrefix(i.Path, vm.Spec.VirtualMachineName) {
				i.Path = vm.Name + "-" + vm.Namespace + "-" + i.Path
			}

			busType := detectDiskBusType(i.DeviceId, vm.GetDefaultDiskBusType())

			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.virtualMachineName": vm.Spec.VirtualMachineName,
				"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
				"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
				"deviceId":                i.DeviceId,
				"path":                    i.Path,
				"busType":                 busType,
				"size":                    i.Size,
			}).Info("Downloading an image")

			exportPath := filepath.Join(tmpPath, i.Path)
			err = lease.DownloadFile(c.ctx, exportPath, i, soap.DefaultDownload)
			if err != nil {
				return err
			}
			vm.Status.DiskImportStatus = append(vm.Status.DiskImportStatus, migration.DiskInfo{
				Name:     i.Path,
				DiskSize: i.Size,
				BusType:  busType,
			})
		} else {
			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.virtualMachineName": vm.Spec.VirtualMachineName,
				"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
				"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
				"deviceId":                i.DeviceId,
				"path":                    i.Path,
				"size":                    i.Size,
			}).Info("Skipping an image")
		}
	}

	u.Done()
	// complete lease since disks have been downloaded
	// and all subsequence processing is local
	// we ignore the error since we have the disks and can continue conversion
	err = lease.Complete(c.ctx)
	if err != nil {
		logrus.Errorf("error marking lease complete: %v", err)
	}

	// disk info will name of disks including the format suffix ".vmdk"
	// once the disks are converted this needs to be updated to ".img"
	// spec for how download_url is generated
	// 				Spec: harvesterv1beta1.VirtualMachineImageSpec{
	//					DisplayName: fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name),
	//					URL: fmt.Sprintf("http://%s:%d/%s.img", server.Address(), server.DefaultPort(), d.Name),
	//				},

	// qemu conversion to raw image file
	// converted disks need to be placed in the server.TmpDir from where they will be served
	for i, d := range vm.Status.DiskImportStatus {
		sourceFile := filepath.Join(tmpPath, d.Name)
		rawDiskName := strings.Split(d.Name, ".vmdk")[0] + ".img"
		destFile := filepath.Join(server.TempDir(), rawDiskName)
		err = qemu.ConvertVMDKtoRAW(sourceFile, destFile)
		if err != nil {
			return fmt.Errorf("error during conversion of VMDK to RAW disk: %v", err)
		}
		// update fields to reflect final location of raw image file
		vm.Status.DiskImportStatus[i].DiskLocalPath = server.TempDir()
		vm.Status.DiskImportStatus[i].Name = rawDiskName
	}
	return nil
}

func (c *Client) ShutdownGuest(vm *migration.VirtualMachineImport) error {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return fmt.Errorf("error finding VM in ShutdownGuest: %w", err)
	}

	ok, err := c.IsPoweredOff(vm)
	if err != nil {
		return err
	}

	if !ok {
		return vmObj.ShutdownGuest(c.ctx)
	}

	return nil
}

func (c *Client) PowerOff(vm *migration.VirtualMachineImport) error {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return fmt.Errorf("error finding VM in PowerOff: %w", err)
	}

	ok, err := c.IsPoweredOff(vm)
	if err != nil {
		return err
	}

	if !ok {
		_, err = vmObj.PowerOff(c.ctx)
		return err
	}

	return nil
}

func (c *Client) IsPowerOffSupported() bool {
	return true
}

func (c *Client) IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error) {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return false, fmt.Errorf("error find VM in IsPoweredOff: %v", err)
	}

	state, err := vmObj.PowerState(c.ctx)
	if err != nil {
		return false, fmt.Errorf("error looking up powerstate: %v", err)
	}

	return state == types.VirtualMachinePowerStatePoweredOff, nil
}

func (c *Client) GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error) {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return nil, fmt.Errorf("error querying vm in GenerateVirtualMachine: %w", err)
	}

	newVM := &kubevirt.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.Status.ImportedVirtualMachineName,
			Namespace: vm.Namespace,
		},
	}

	var o mo.VirtualMachine

	err = vmObj.Properties(c.ctx, vmObj.Reference(), []string{}, &o)
	if err != nil {
		return nil, err
	}

	// Log the origin VM specification for better troubleshooting.
	// Note, JSON is used to be able to prettify the output for better readability.
	logrus.WithFields(util.FieldsToJSON(logrus.Fields{
		"name":      vm.Name,
		"namespace": vm.Namespace,
		"spec":      o,
	}, []string{"spec"})).Info("Origin spec of the VM to be imported")

	// Need CPU, Socket, Memory, VirtualNIC information to perform the mapping
	networkInfos := generateNetworkInfos(c.networkMapping, o.Config.Hardware.Device)

	vmSpec := kubevirt.VirtualMachineSpec{
		RunStrategy: &[]kubevirt.VirtualMachineRunStrategy{kubevirt.RunStrategyRerunOnFailure}[0],
		Template: &kubevirt.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"harvesterhci.io/vmName": vm.Status.ImportedVirtualMachineName,
				},
			},
			Spec: kubevirt.VirtualMachineInstanceSpec{
				Domain: kubevirt.DomainSpec{
					CPU: &kubevirt.CPU{
						Cores:   uint32(o.Config.Hardware.NumCPU),            // nolint:gosec
						Sockets: uint32(o.Config.Hardware.NumCoresPerSocket), // nolint:gosec
						Threads: 1,
					},
					Memory: &kubevirt.Memory{
						Guest: &[]resource.Quantity{resource.MustParse(fmt.Sprintf("%dM", o.Config.Hardware.MemoryMB))}[0],
					},
					Resources: kubevirt.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dM", o.Config.Hardware.MemoryMB)),
							corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", o.Config.Hardware.NumCPU)),
						},
					},
					Features: &kubevirt.Features{
						ACPI: kubevirt.FeatureState{
							Enabled: ptr.To(true),
						},
					},
				},
			},
		},
	}

	mappedNetwork := source.MapNetworks(networkInfos, vm.Spec.Mapping)
	networkConfig, interfaceConfig := source.GenerateNetworkInterfaceConfigs(mappedNetwork, vm.GetDefaultNetworkInterfaceModel())

	// Setup BIOS/EFI, SecureBoot and TPM settings.
	uefi := strings.EqualFold(o.Config.Firmware, string(types.GuestOsDescriptorFirmwareTypeEfi))
	secureBoot := false
	if o.Config.BootOptions != nil {
		secureBoot = ptr.Deref(o.Config.BootOptions.EfiSecureBootEnabled, false)
	}
	tpm := ptr.Deref(o.Summary.Config.TpmPresent, false)
	if uefi {
		source.VMSpecSetupUEFISettings(&vmSpec, secureBoot, tpm)
	}

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaceConfig
	newVM.Spec = vmSpec

	// disk attachment needs query by core controller for storage classes, so will be added by the migration controller
	return newVM, nil
}

func (c *Client) findVM(path, name string) (*object.VirtualMachine, error) {
	f := find.NewFinder(c.Client.Client, true)
	dc := c.dc
	if !strings.HasPrefix(c.dc, "/") {
		dc = fmt.Sprintf("/%s", c.dc)
	}
	vmPath := filepath.Join(dc, "/vm", path, name)
	return f.VirtualMachine(c.ctx, vmPath)
}

func generateNetworkInfos(networkMap map[string]string, devices []types.BaseVirtualDevice) []source.NetworkInfo {
	result := make([]source.NetworkInfo, 0, len(devices))

	for _, d := range devices {
		switch d := d.(type) {
		case *types.VirtualVmxnet:
			obj := d
			summary := identifyNetworkName(networkMap, *obj.GetVirtualDevice())
			if summary == "" {
				summary = obj.DeviceInfo.GetDescription().Summary
			}
			result = append(result, source.NetworkInfo{
				NetworkName: summary,
				MAC:         obj.MacAddress,
				Model:       migration.NetworkInterfaceModelVirtio,
			})
		case *types.VirtualE1000e:
			obj := d
			summary := identifyNetworkName(networkMap, *obj.GetVirtualDevice())
			if summary == "" {
				summary = obj.DeviceInfo.GetDescription().Summary
			}
			result = append(result, source.NetworkInfo{
				NetworkName: summary,
				MAC:         obj.MacAddress,
				Model:       migration.NetworkInterfaceModelE1000e,
			})
		case *types.VirtualE1000:
			obj := d
			summary := identifyNetworkName(networkMap, *obj.GetVirtualDevice())
			if summary == "" {
				summary = obj.DeviceInfo.GetDescription().Summary
			}
			result = append(result, source.NetworkInfo{
				NetworkName: summary,
				MAC:         obj.MacAddress,
				Model:       migration.NetworkInterfaceModelE1000,
			})
		case *types.VirtualVmxnet3:
			obj := d
			summary := identifyNetworkName(networkMap, *obj.GetVirtualDevice())
			if summary == "" {
				summary = obj.DeviceInfo.GetDescription().Summary
			}
			result = append(result, source.NetworkInfo{
				NetworkName: summary,
				MAC:         obj.MacAddress,
				Model:       migration.NetworkInterfaceModelVirtio,
			})
		case *types.VirtualVmxnet2:
			obj := d
			summary := identifyNetworkName(networkMap, *obj.GetVirtualDevice())
			if summary == "" {
				summary = obj.DeviceInfo.GetDescription().Summary
			}
			result = append(result, source.NetworkInfo{
				NetworkName: summary,
				MAC:         obj.MacAddress,
				Model:       migration.NetworkInterfaceModelVirtio,
			})
		case *types.VirtualPCNet32:
			obj := d
			summary := identifyNetworkName(networkMap, *obj.GetVirtualDevice())
			if summary == "" {
				summary = obj.DeviceInfo.GetDescription().Summary
			}
			result = append(result, source.NetworkInfo{
				NetworkName: summary,
				MAC:         obj.MacAddress,
				Model:       migration.NetworkInterfaceModelPcnet,
			})
		}
	}

	return result
}

// detectDiskBusType tries to identify the disk bus type from VMware to attempt and
// set correct bus types in KubeVirt. Defaults to the specified bus type in `def`
// if auto-detection fails.
// Examples:
// .--------------------------------------------------.
// | Bus  | Device ID                                 |
// |------|-------------------------------------------|
// | SCSI | /vm-13010/ParaVirtualSCSIController0:0    |
// | SCSI | /vm-13011/VirtualBusLogicController0:0    |
// | SCSI | /vm-13012/VirtualLsiLogicController0:0    |
// | SCSI | /vm-13013/VirtualLsiLogicSASController0:0 |
// | SATA | /vm-13767/VirtualAHCIController0:1        |
// | IDE  | /vm-5678/VirtualIDEController1:0          |
// | NVMe | /vm-2468/VirtualNVMEController0:0         |
// | USB  | /vm-54321/VirtualUSBController0:0         |
// '--------------------------------------------------'
// References:
// - https://github.com/vmware/pyvmomi/tree/master/pyVmomi/vim/vm/device
// - https://vdc-download.vmware.com/vmwb-repository/dcr-public/d1902b0e-d479-46bf-8ac9-cee0e31e8ec0/07ce8dbd-db48-4261-9b8f-c6d3ad8ba472/vim.vm.device.VirtualSCSIController.html
// - https://libvirt.org/formatdomain.html#controllers
// - https://kubevirt.io/api-reference/v1.1.0/definitions.html#_v1_disktarget
func detectDiskBusType(deviceID string, def kubevirt.DiskBus) kubevirt.DiskBus {
	deviceID = strings.ToLower(deviceID)
	switch {
	case strings.Contains(deviceID, "paravirtualscsi"):
		// The pvscsi (Paravirtual SCSI) controller cannot be mapped to
		// SCSI bus type because in KubeVirt it is not possible to specify
		// the exact model (pvscsi, lsilogic, ...) of the disk via the
		// VirtualMachine API. Attempting to map pvscsi to SCSI prevents
		// the VM from booting.
		// As a workaround, the SATA bus type is utilized in such case.
		// Note, VirtIO would be better, but it is not said that the
		// required drivers are installed in the VM.
		return kubevirt.DiskBusSATA
	case strings.Contains(deviceID, "scsi"), strings.Contains(deviceID, "buslogic"), strings.Contains(deviceID, "lsilogic"):
		return kubevirt.DiskBusSCSI
	case strings.Contains(deviceID, "ahci"), strings.Contains(deviceID, "sata"), strings.Contains(deviceID, "ide"):
		return kubevirt.DiskBusSATA
	case strings.Contains(deviceID, "nvme"):
		return kubevirt.DiskBusVirtio
	case strings.Contains(deviceID, "usb"):
		return kubevirt.DiskBusUSB
	default:
		return def
	}
}

// SanitizeVirtualMachineImport is used to sanitize the VirtualMachineImport object.
func (c *Client) SanitizeVirtualMachineImport(vm *migration.VirtualMachineImport) error {
	// Note, VMware allows upper case characters in virtual machine names,
	// so we need to convert them to lower case to be RFC 1123 compliant.
	vm.Status.ImportedVirtualMachineName = strings.ToLower(vm.Spec.VirtualMachineName)

	return nil
}

// GenerateNetworkMayByRef lists all networks defined in the DC and converts them to
// network id: network name
// this subsequently used to map a network id to network name if needed based on the type
// of backing device for a nic
func GenerateNetworkMapByRef(ctx context.Context, c *vim25.Client) (map[string]string, error) {
	networks, err := generateNetworkList(ctx, c)
	if err != nil {
		return nil, err
	}
	returnMap := make(map[string]string, len(networks))
	for _, v := range networks {
		returnMap[v.Reference().Value] = v.Name
	}
	logrus.Debugf("generated networkMapByRef: %v", returnMap)
	return returnMap, nil
}

func GenerateNetworkMapByName(ctx context.Context, c *vim25.Client) (map[string]string, error) {
	networks, err := generateNetworkList(ctx, c)
	if err != nil {
		return nil, err
	}
	returnMap := make(map[string]string, len(networks))
	for _, v := range networks {
		returnMap[v.Name] = v.Reference().Value
	}
	logrus.Debugf("generated networkMapByName: %v", returnMap)
	return returnMap, nil
}

func generateNetworkList(ctx context.Context, c *vim25.Client) ([]mo.Network, error) {
	var networks []mo.Network
	manager := view.NewManager(c)
	networkView, err := manager.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"Network"}, true)
	if err != nil {
		return networks, fmt.Errorf("error generating network container view: %v", err)
	}
	defer networkView.Destroy(ctx)
	if err := networkView.Retrieve(ctx, []string{"Network"}, nil, &networks); err != nil {
		return networks, fmt.Errorf("error retreiving networks: %v", err)
	}
	return networks, nil
}

// identifyNetworkName uses the backing device for a nic to identify network name correctly
// in case of a nic using a Distributed VSwitch the summary returned from device is of the form
// DVSwitch : HEX NUMBER which breaks network lookup. As a result we need to identify the network name
// from the PortGroupKey
func identifyNetworkName(networkMap map[string]string, device types.VirtualDevice) string {
	var summary string
	backing := device.Backing
	switch b := backing.(type) {
	case *types.VirtualEthernetCardDistributedVirtualPortBackingInfo:
		obj := b
		logrus.Debugf("looking up portgroupkey: %v", obj.Port.PortgroupKey)
		summary = networkMap[obj.Port.PortgroupKey]
	case *types.VirtualEthernetCardNetworkBackingInfo:
		obj := b
		logrus.Debugf("using devicename: %v", obj.DeviceName)
		summary = obj.DeviceName
	default:
		summary = ""
	}
	return summary
}

func (c *Client) ListNetworks() error {
	mgr := view.NewManager(c.Client.Client)

	v, err := mgr.CreateContainerView(c.ctx, c.ServiceContent.RootFolder, []string{"Network"}, true)
	if err != nil {
		return fmt.Errorf("error creating view %v", err)
	}

	defer v.Destroy(c.ctx)
	var networks []mo.Network
	err = v.Retrieve(c.ctx, []string{"Network"}, nil, &networks)
	if err != nil {
		return fmt.Errorf("error fetching networks: %v", err)
	}

	for _, net := range networks {
		fmt.Printf("%s: %s\n", net.Name, net.Reference())
	}

	return nil
}
