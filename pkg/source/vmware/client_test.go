package vmware

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirt "kubevirt.io/api/core/v1"
	"k8s.io/utils/pointer"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
)

var vcsimPort string

// setup mock vmware endpoint
func TestMain(t *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("error connecting to dockerd: %v", err)
	}

	// https://hub.docker.com/r/vmware/vcsim
	runOpts := &dockertest.RunOptions{
		Name:       "vcsim",
		Repository: "vmware/vcsim",
		Tag:        "v0.49.0",
	}

	vcsimMock, err := pool.RunWithOptions(runOpts)

	if err != nil {
		log.Fatalf("error creating vcsim container: %v", err)
	}

	vcsimPort = vcsimMock.GetPort("8989/tcp")
	time.Sleep(30 * time.Second)
	go func() {
		if err = server.NewServer(context.TODO()); err != nil {
			log.Fatalf("error creating server: %v", err)
		}
	}()

	code := t.Run()
	if err := pool.Purge(vcsimMock); err != nil {
		log.Fatalf("error purging vcsimMock container: %v", err)
	}

	os.Exit(code)
}

func Test_NewClient(t *testing.T) {
	ctx := context.TODO()
	endpoint := fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
	dc := "DC0"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	c, err := NewClient(ctx, endpoint, dc, secret)
	assert := require.New(t)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")
}

func Test_PowerOffVirtualMachine(t *testing.T) {
	ctx := context.TODO()
	endpoint := fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
	dc := "DC0"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	c, err := NewClient(ctx, endpoint, dc, secret)
	assert := require.New(t)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	vm := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: "DC0_H0_VM0",
		},
	}

	err = c.PowerOffVirtualMachine(vm)
	assert.NoError(err, "expected no error during VM power off")
}

func Test_IsPoweredOff(t *testing.T) {
	ctx := context.TODO()
	endpoint := fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
	dc := "DC0"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	c, err := NewClient(ctx, endpoint, dc, secret)
	assert := require.New(t)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	vm := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: "DC0_H0_VM0",
		},
	}

	ok, err := c.IsPoweredOff(vm)
	assert.NoError(err, "expected no error during check for power status")
	assert.True(ok, "expected machine to be powered")
}

// Test_ExportVirtualMachine needs to reference a real vcenter as the vcsim doesnt support ovf export functionality
func Test_ExportVirtualMachine(t *testing.T) {
	// skip as vscim doesnt implement the same
	_, ok := os.LookupEnv("USE_EXISTING")
	if !ok {
		return
	}

	ctx := context.TODO()
	assert := require.New(t)
	govcURL := os.Getenv("GOVC_URL")
	assert.NotEmpty(govcURL, "expected govcURL to be set")
	govcDatacenter := os.Getenv("GOVC_DATACENTER")
	assert.NotEmpty(govcDatacenter, "expected govcDatacenter to be set")

	data := make(map[string]string)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	govcUsername := os.Getenv("GOVC_USERNAME")
	assert.NotEmpty(govcUsername, "expected govcUsername to be set")
	data["username"] = govcUsername

	govcPassword := os.Getenv("GOVC_PASSWORD")
	assert.NotEmpty(govcPassword, "expected govcPassword to be set")
	data["password"] = govcPassword
	secret.StringData = data

	vmName := os.Getenv("VM_NAME")
	assert.NotEmpty(vmName, "expected vmName to be set")

	c, err := NewClient(ctx, govcURL, govcDatacenter, secret)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	vm := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: vmName,
		},
	}

	err = c.ExportVirtualMachine(vm)
	assert.NoError(err, "expected no error during VM export")
	t.Log(vm.Status)
}

// Test_GenerateVirtualMachine
func Test_GenerateVirtualMachine(t *testing.T) {
	ctx := context.TODO()

	endpoint := fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
	dc := "DC0"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	c, err := NewClient(ctx, endpoint, dc, secret)
	assert := require.New(t)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	vm := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: "DC0_H0_VM0",
			Mapping: []migration.NetworkMapping{
				{
					SourceNetwork:      "DVSwitch: fea97929-4b2d-5972-b146-930c6d0b4014",
					DestinationNetwork: "default/vlan",
				},
			},
		},
	}

	newVM, err := c.GenerateVirtualMachine(vm)
	assert.NoError(err, "expected no error during VM CR generation")
	assert.Len(newVM.Spec.Template.Spec.Networks, 1, "should have found the default pod network")
	assert.Len(newVM.Spec.Template.Spec.Domain.Devices.Interfaces, 1, "should have found a network map")
	assert.Equal(newVM.Spec.Template.Spec.Domain.Memory.Guest.String(), "32M", "expected VM to have 32M memory")
	assert.NotEmpty(newVM.Spec.Template.Spec.Domain.Resources.Limits, "expect to find resource requests to be present")
	assert.Equal(newVM.Spec.Template.Spec.Domain.Devices.Interfaces[0].Model, migration.NetworkInterfaceModelE1000, "expected to have a NIC with e1000 model")
}

func Test_GenerateVirtualMachine_secureboot(t *testing.T) {
	assert := require.New(t)
	ctx := context.TODO()
	endpoint := fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}
	vm := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: "test01",
			Mapping: []migration.NetworkMapping{
				{
					SourceNetwork:      "DVSwitch: fea97929-4b2d-5972-b146-930c6d0b4014",
					DestinationNetwork: "default/vlan",
				},
			},
		},
	}

	c, err := NewClient(ctx, endpoint, "DC0", secret)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	// https://github.com/vmware/govmomi/blob/main/vcsim/README.md#default-vcenter-inventory
	f := find.NewFinder(c.Client.Client, true)

	dc, err := f.Datacenter(ctx, c.dc)
	assert.NoError(err, "expected no error during datacenter lookup")

	f.SetDatacenter(dc)

	ds, err := f.DefaultDatastore(ctx)
	assert.NoError(err, "expected no error during datastore lookup")

	pool, err := f.ResourcePool(ctx, "DC0_H0/Resources")
	assert.NoError(err, "expected no error during resource pool lookup")

	folder, err := dc.Folders(ctx)
	assert.NoError(err, "expected no error during folder lookup")

	vmConfigSpec := types.VirtualMachineConfigSpec{
		Name:     vm.Spec.VirtualMachineName,
		GuestId:  string(types.VirtualMachineGuestOsIdentifierOtherGuest64),
		Firmware: string(types.GuestOsDescriptorFirmwareTypeEfi),
		BootOptions: &types.VirtualMachineBootOptions{
			EfiSecureBootEnabled: pointer.Bool(true),
		},
		Files: &types.VirtualMachineFileInfo{
			VmPathName: fmt.Sprintf("[%s] %s", ds.Name(), vm.Spec.VirtualMachineName),
		},
	}

	task, err := folder.VmFolder.CreateVM(ctx, vmConfigSpec, pool, nil)
	assert.NoError(err, "expected no error when creating VM")

	_, err = task.WaitForResult(ctx, nil)
	assert.NoError(err, "expected no error when waiting for task to complete")

	newVM, err := c.GenerateVirtualMachine(vm)
	assert.NoError(err, "expected no error during VM CR generation")
	assert.True(*newVM.Spec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot, "expected VM to have secure boot enabled")
	assert.True(*newVM.Spec.Template.Spec.Domain.Features.SMM.Enabled, "expected VM to have SMM enabled")
}

func Test_identifyNetworkCards(t *testing.T) {
	ctx := context.TODO()
	endpoint := fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
	dc := "DC0"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	c, err := NewClient(ctx, endpoint, dc, secret)
	assert := require.New(t)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	vmObj, err := c.findVM("", "DC0_H0_VM0")
	assert.NoError(err, "expected no error during VM lookup")

	var o mo.VirtualMachine

	err = vmObj.Properties(c.ctx, vmObj.Reference(), []string{}, &o)
	assert.NoError(err, "expected no error looking up vmObj properties")

	networkInfo := generateNetworkInfos(o.Config.Hardware.Device)
	assert.Len(networkInfo, 1, "expected to find only 1 item in the networkInfo")
	networkMapping := []migration.NetworkMapping{
		{
			SourceNetwork:      "dummyNetwork",
			DestinationNetwork: "harvester1",
		},
		{
			SourceNetwork:         "DVSwitch: fea97929-4b2d-5972-b146-930c6d0b4014",
			DestinationNetwork:    "pod-network",
			NetworkInterfaceModel: pointer.String(migration.NetworkInterfaceModelRtl8139),
		},
	}

	mappedInfo := source.MapNetworks(networkInfo, networkMapping)
	assert.Len(mappedInfo, 1, "expected to find only 1 item in the mapped networkinfo")
	assert.Equal(mappedInfo[0].Model, "rtl8139", "expected to have a NIC with rtl8139 model")

	noNetworkMapping := []migration.NetworkMapping{}
	noMappedInfo := source.MapNetworks(networkInfo, noNetworkMapping)
	assert.Len(noMappedInfo, 0, "expected to find no item in the mapped networkinfo")
}

func Test_adapterType(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		deviceID string
		expected kubevirt.DiskBus
	}{
		{
			desc:     "SCSI disk",
			deviceID: "/vm-13010/ParaVirtualSCSIController0:0",
			expected: kubevirt.DiskBusSCSI,
		},
		{
			desc:     "NVMe disk",
			deviceID: "/vm-2468/VirtualNVMEController0:0",
			expected: kubevirt.DiskBusVirtio,
		},
		{
			desc:     "USB disk",
			deviceID: "/vm-54321/VirtualUSBController0:0",
			expected: kubevirt.DiskBusUSB,
		},
		{
			desc:     "SATA disk",
			deviceID: "/vm-13767/VirtualAHCIController0:1",
			expected: kubevirt.DiskBusSATA,
		},
		{
			desc:     "IDE disk",
			deviceID: "/vm-5678/VirtualIDEController1:0",
			expected: kubevirt.DiskBusSATA,
		},
		{
			desc:     "Unknown disk",
			deviceID: "foo",
			expected: kubevirt.DiskBusVirtio,
		},
	}

	for _, tc := range testCases {
		busType := detectBusType(tc.deviceID)
		assert.Equal(tc.expected, busType)
	}
}
