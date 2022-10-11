package vmware

import (
	"context"
	"fmt"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/harvester/vm-import-controller/pkg/qemu"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirt "kubevirt.io/api/core/v1"
)

type Client struct {
	ctx context.Context
	*govmomi.Client
	tmpCerts string
	dc       string
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

	endpointUrl, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error parsing endpoint url: %v", err)
	}

	sc := soap.NewClient(endpointUrl, insecure)

	vmwareClient := &Client{}
	if !insecure {
		tmpFile, err := ioutil.TempFile("/tmp", "vmware-ca-")
		if err != nil {
			return nil, fmt.Errorf("error creating tmp file for vmware ca certs: %v", err)
		}
		_, err = tmpFile.Write(caCert)
		if err != nil {
			return nil, fmt.Errorf("error writing ca cert to tmp file %s: %v", tmpFile.Name(), err)
		}
		sc.SetRootCAs(tmpFile.Name())
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

// Verify checks is a verfication check for migration provider to ensure that the config is valid
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

	logrus.Infof("found dc :%v", dcObj)
	return nil
}

func (c *Client) ExportVirtualMachine(vm *migration.VirtualMachineImport) error {

	tmpPath, err := ioutil.TempDir("/tmp", fmt.Sprintf("%s-%s-", vm.Name, vm.Namespace))

	if err != nil {
		return fmt.Errorf("error creating tmp dir for vmexport: %v", err)
	}

	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return fmt.Errorf("error finding vm in ExportVirtualMacine: %v", err)
	}

	lease, err := vmObj.Export(c.ctx)
	if err != nil {
		return fmt.Errorf("error generate export lease in ExportVirtualMachine: %v", err)
	}

	info, err := lease.Wait(c.ctx, nil)
	if err != nil {
		return err
	}

	u := lease.StartUpdater(c.ctx, info)
	defer u.Done()
	defer lease.Complete(c.ctx)

	for _, i := range info.Items {
		// ignore iso and nvram disks
		if strings.HasSuffix(i.Path, ".vmdk") {
			if !strings.HasPrefix(i.Path, vm.Spec.VirtualMachineName) {
				i.Path = vm.Name + "-" + vm.Namespace + "-" + i.Path
			}

			exportPath := filepath.Join(tmpPath, i.Path)
			err = lease.DownloadFile(c.ctx, exportPath, i, soap.DefaultDownload)
			if err != nil {
				return err
			}
			vm.Status.DiskImportStatus = append(vm.Status.DiskImportStatus, migration.DiskInfo{
				Name:     i.Path,
				DiskSize: i.Size,
			})
		}
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
		err := qemu.ConvertVMDKtoRAW(sourceFile, destFile)
		if err != nil {
			return err
		}
		// update fields to reflect final location of raw image file
		vm.Status.DiskImportStatus[i].DiskLocalPath = server.TempDir()
		vm.Status.DiskImportStatus[i].Name = rawDiskName
	}

	return os.RemoveAll(tmpPath)
}

func (c *Client) PowerOffVirtualMachine(vm *migration.VirtualMachineImport) error {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return fmt.Errorf("error finding vm in PowerOffVirtualMachine: %v", err)
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

func (c *Client) IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error) {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return false, fmt.Errorf("error find VM in IsPoweredOff :%v", err)
	}

	state, err := vmObj.PowerState(c.ctx)
	if err != nil {
		return false, fmt.Errorf("error looking up powerstate: %v", err)
	}

	if state == types.VirtualMachinePowerStatePoweredOff {
		return true, nil
	}

	return false, nil
}

func (c *Client) GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error) {
	vmObj, err := c.findVM(vm.Spec.Folder, vm.Spec.VirtualMachineName)
	if err != nil {
		return nil, fmt.Errorf("error quering vm in GenerateVirtualMachine: %v", err)
	}
	newVM := &kubevirt.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.Spec.VirtualMachineName,
			Namespace: vm.Namespace,
		},
	}

	var o mo.VirtualMachine

	err = vmObj.Properties(c.ctx, vmObj.Reference(), []string{}, &o)
	if err != nil {
		return nil, err
	}

	// Need CPU, Socket, Memory, VirtualNIC information to perform the mapping
	networkInfo := identifyNetworkCards(o.Config.Hardware.Device)

	vmSpec := kubevirt.VirtualMachineSpec{
		RunStrategy: &[]kubevirt.VirtualMachineRunStrategy{kubevirt.RunStrategyRerunOnFailure}[0],
		Template: &kubevirt.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"harvesterhci.io/vmName": vm.Spec.VirtualMachineName,
				},
			},
			Spec: kubevirt.VirtualMachineInstanceSpec{
				Domain: kubevirt.DomainSpec{
					CPU: &kubevirt.CPU{
						Cores:   uint32(o.Config.Hardware.NumCPU),
						Sockets: uint32(o.Config.Hardware.NumCoresPerSocket),
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
				},
			},
		},
	}

	var networkConfig []kubevirt.Network

	mappedNetwork := mapNetworkCards(networkInfo, vm.Spec.Mapping)
	for i, v := range mappedNetwork {
		networkConfig = append(networkConfig, kubevirt.Network{
			NetworkSource: kubevirt.NetworkSource{
				Multus: &kubevirt.MultusNetwork{
					NetworkName: v.MappedNetwork,
				},
			},
			Name: fmt.Sprintf("migrated-%d", i),
		})
	}

	var interfaces []kubevirt.Interface
	for i, v := range mappedNetwork {
		interfaces = append(interfaces, kubevirt.Interface{
			Name:       fmt.Sprintf("migrated-%d", i),
			MacAddress: v.MAC,
			Model:      "virtio",
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Bridge: &kubevirt.InterfaceBridge{},
			},
		})
	}
	// if there is no network, attach to Pod network. Essential for VM to be booted up
	if len(networkConfig) == 0 {
		networkConfig = append(networkConfig, kubevirt.Network{
			Name: "pod-network",
			NetworkSource: kubevirt.NetworkSource{
				Pod: &kubevirt.PodNetwork{},
			},
		})
		interfaces = append(interfaces, kubevirt.Interface{
			Name:  "pod-network",
			Model: "virtio",
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Masquerade: &kubevirt.InterfaceMasquerade{},
			},
		})
	}

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaces
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

type networkInfo struct {
	NetworkName   string
	MAC           string
	MappedNetwork string
}

func identifyNetworkCards(devices []types.BaseVirtualDevice) []networkInfo {
	var resp []networkInfo
	for _, d := range devices {
		switch d.(type) {
		case *types.VirtualVmxnet:
			obj := d.(*types.VirtualVmxnet)
			resp = append(resp, networkInfo{
				NetworkName: obj.DeviceInfo.GetDescription().Summary,
				MAC:         obj.MacAddress,
			})
		case *types.VirtualE1000e:
			obj := d.(*types.VirtualE1000e)
			resp = append(resp, networkInfo{
				NetworkName: obj.DeviceInfo.GetDescription().Summary,
				MAC:         obj.MacAddress,
			})
		case *types.VirtualE1000:
			obj := d.(*types.VirtualE1000)
			resp = append(resp, networkInfo{
				NetworkName: obj.DeviceInfo.GetDescription().Summary,
				MAC:         obj.MacAddress,
			})
		case *types.VirtualVmxnet3:
			obj := d.(*types.VirtualVmxnet3)
			resp = append(resp, networkInfo{
				NetworkName: obj.DeviceInfo.GetDescription().Summary,
				MAC:         obj.MacAddress,
			})
		case *types.VirtualVmxnet2:
			obj := d.(*types.VirtualVmxnet2)
			resp = append(resp, networkInfo{
				NetworkName: obj.DeviceInfo.GetDescription().Summary,
				MAC:         obj.MacAddress,
			})
		}
	}

	return resp
}

func mapNetworkCards(networkCards []networkInfo, mapping []migration.NetworkMapping) []networkInfo {
	var retNetwork []networkInfo
	for _, nc := range networkCards {
		for _, m := range mapping {
			if m.SourceNetwork == nc.NetworkName {
				nc.MappedNetwork = m.DestinationNetwork
				retNetwork = append(retNetwork, nc)
			}
		}
	}

	return retNetwork
}
