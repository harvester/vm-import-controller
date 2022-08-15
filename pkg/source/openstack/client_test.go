package openstack

import (
	"context"
	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

var (
	c      *Client
	testVM string
)

func TestMain(t *testing.M) {
	var err error
	s, err := SetupOpenstackSecretFromEnv("devstack")
	if err != nil {
		logrus.Fatal(err)
	}
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	if err != nil {
		logrus.Fatal(err)
	}

	c, err = NewClient(context.TODO(), endpoint, region, s)
	if err != nil {
		logrus.Fatal(err)
	}

	go func() {
		server.NewServer(context.TODO())
	}()

	if err != nil {
		logrus.Fatal(err)
	}

	code := t.Run()
	os.Exit(code)
}
func Test_NewClient(t *testing.T) {
	assert := require.New(t)
	err := c.Verify()
	assert.NoError(err, "expect no error during verify of client")
}

func Test_checkOrGetUUID(t *testing.T) {
	assert := require.New(t)
	vmName, ok := os.LookupEnv("OS_VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	_, err := c.checkOrGetUUID(vmName)
	assert.NoError(err, "expected no error during checkOrGetUUID")
}

func Test_IsPoweredOff(t *testing.T) {
	assert := require.New(t)
	vmName, ok := os.LookupEnv("OS_VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	_, err := c.IsPoweredOff(vm)
	assert.NoError(err, "expected no error during check of power status")
}

func Test_PowerOffVirtualMachine(t *testing.T) {
	assert := require.New(t)
	vmName, ok := os.LookupEnv("OS_VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	err := c.PowerOffVirtualMachine(vm)
	assert.NoError(err, "expected no error during check of power status")
}

func Test_ExportVirtualMachine(t *testing.T) {
	assert := require.New(t)
	vmName, ok := os.LookupEnv("OS_VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	err := c.ExportVirtualMachine(vm)
	assert.NoError(err, "expected no error during exportvirtualmachines")
	assert.NotEmpty(vm.Status.DiskImportStatus, "expected diskimportstatus to be populated")
	t.Log(vm.Status.DiskImportStatus)
}

func Test_GenerateVirtualMachine(t *testing.T) {
	assert := require.New(t)
	vmName := os.Getenv("OS_VM_NAME")
	assert.NotEmpty(vmName, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	newVM, err := c.GenerateVirtualMachine(vm)
	assert.NoError(err, "expected no error during GenerateVirtualMachine")
	assert.NotEmpty(newVM.Spec.Template.Spec.Domain.CPU, "expected CPU's to not be empty")
	assert.NotEmpty(newVM.Spec.Template.Spec.Domain.Resources.Limits.Memory(), "expected memory limit to not be empty")
	assert.NotEmpty(newVM.Spec.Template.Spec.Networks, "expected to find atleast 1 network as pod network should have been applied")
	assert.NotEmpty(newVM.Spec.Template.Spec.Domain.Devices.Interfaces, "expected to find atleast 1 interface for pod-network")
}
