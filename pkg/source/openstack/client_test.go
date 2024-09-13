package openstack

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
)

var (
	c *Client
)

func TestMain(t *testing.M) {
	var err error

	// skip tests, needed for current builds
	_, ok := os.LookupEnv("USE_EXISTING_CLUSTER")
	if !ok {
		logrus.Warn("skipping tests")
		return
	}

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
		if err = server.NewServer(context.TODO()); err != nil {
			logrus.Fatalf("error creating server: %v", err)
		}
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
	vm := &migration.VirtualMachineImport{
		Spec: migration.VirtualMachineImportSpec{
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
	vm := &migration.VirtualMachineImport{
		Spec: migration.VirtualMachineImportSpec{
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
	vm := &migration.VirtualMachineImport{
		Spec: migration.VirtualMachineImportSpec{
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
	vm := &migration.VirtualMachineImport{
		Spec: migration.VirtualMachineImportSpec{
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

func Test_generateNetworkInfo(t *testing.T) {
	networkInfoByte := []byte(`{"private":[{"OS-EXT-IPS-MAC:mac_addr":"fa:16:3e:92:5f:45","OS-EXT-IPS:type":"fixed","addr":"fd5b:731d:94e1:0:f816:3eff:fe92:5f45","version":6},{"OS-EXT-IPS-MAC:mac_addr":"fa:16:3e:92:5f:45","OS-EXT-IPS:type":"fixed","addr":"10.0.0.38","version":4}],"shared":[{"OS-EXT-IPS-MAC:mac_addr":"fa:16:3e:ec:49:11","OS-EXT-IPS:type":"fixed","addr":"192.168.233.233","version":4}]}`)
	var networkInfoMap map[string]interface{}
	assert := require.New(t)
	err := json.Unmarshal(networkInfoByte, &networkInfoMap)
	assert.NoError(err, "expected no error while unmarshalling network info")

	vmInterfaceDetails, err := generateNetworkInfo(networkInfoMap)
	assert.NoError(err, "expected no error while generating network info")
	assert.Len(vmInterfaceDetails, 2, "expected to find 2 interfaces only")

}
