package openstack

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
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

	source := migration.OpenstackSource{}
	options := source.GetOptions().(migration.OpenstackSourceOptions)

	c, err = NewClient(context.TODO(), endpoint, region, s, options)
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

func Test_ClientOptions(t *testing.T) {
	assert := require.New(t)
	assert.Equal(c.options.UploadImageRetryCount, migration.OpenstackDefaultRetryCount)
	assert.Equal(c.options.UploadImageRetryDelay, migration.OpenstackDefaultRetryDelay)
}

func Test_SourceGetOptions(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		options  migration.OpenstackSourceOptions
		expected migration.OpenstackSourceOptions
	}{
		{
			desc: "custom count and delay",
			options: migration.OpenstackSourceOptions{
				UploadImageRetryCount: 25,
				UploadImageRetryDelay: 15,
			},
			expected: migration.OpenstackSourceOptions{
				UploadImageRetryCount: 25,
				UploadImageRetryDelay: 15,
			},
		},
		{
			desc: "custom count and default delay",
			options: migration.OpenstackSourceOptions{
				UploadImageRetryCount: 100,
			},
			expected: migration.OpenstackSourceOptions{
				UploadImageRetryCount: 100,
				UploadImageRetryDelay: migration.OpenstackDefaultRetryDelay,
			},
		},
		{
			desc: "default count and custom delay",
			options: migration.OpenstackSourceOptions{
				UploadImageRetryDelay: 50,
			},
			expected: migration.OpenstackSourceOptions{
				UploadImageRetryCount: migration.OpenstackDefaultRetryCount,
				UploadImageRetryDelay: 50,
			},
		},
	}

	for _, tc := range testCases {
		source := migration.OpenstackSource{
			Spec: migration.OpenstackSourceSpec{
				OpenstackSourceOptions: tc.options,
			},
		}
		options := source.GetOptions().(migration.OpenstackSourceOptions)

		assert.Equal(options.UploadImageRetryCount, tc.expected.UploadImageRetryCount, tc.desc)
		assert.Equal(options.UploadImageRetryDelay, tc.expected.UploadImageRetryDelay, tc.desc)
	}
}

func Test_ExtendedServer(t *testing.T) {
	assert := require.New(t)

	var dejson any
	sejson := []byte(`{"server": {"id": "b3693d06-8135-4c7c-b3ea-d37b2cc6fb8f", "name": "cirros-tiny", "status": "SHUTOFF", "tenant_id": "88c800f12d7d4e4e93b2e2883aed1bf5", "user_id": "94ebd4b2c5a140dd9bc20dc5139d6823", "metadata": {}, "hostId": "d44ae638ea333eefe401ae01c9dec9add9ed7b6cad1024a3a220d1f4", "image": "", "flavor": {"id": "1", "links": [{"rel": "bookmark", "href": "http://48.151.623.42/compute/flavors/1"}]}, "created": "2025-02-18T17:07:24Z", "updated": "2025-02-18T17:25:13Z", "addresses": {"shared": [{"version": 4, "addr": "192.168.233.13", "OS-EXT-IPS:type": "fixed", "OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:25:90:74"}]}, "accessIPv4": "", "accessIPv6": "", "links": [{"rel": "self", "href": "http://48.151.623.42/compute/v2.1/servers/b3693d06-8135-4c7c-b3ea-d37b2cc6fb8f"}, {"rel": "bookmark", "href": "http://48.151.623.42/compute/servers/b3693d06-8135-4c7c-b3ea-d37b2cc6fb8f"}], "OS-DCF:diskConfig": "AUTO", "OS-EXT-AZ:availability_zone": "nova", "config_drive": "", "key_name": null, "OS-SRV-USG:launched_at": "2025-02-18T17:08:02.000000", "OS-SRV-USG:terminated_at": null, "OS-EXT-SRV-ATTR:host": "opnstk-server-vm", "OS-EXT-SRV-ATTR:instance_name": "instance-00000002", "OS-EXT-SRV-ATTR:hypervisor_hostname": "opnstk-server-vm", "OS-EXT-SRV-ATTR:reservation_id": "r-j7s0gpwg", "OS-EXT-SRV-ATTR:launch_index": 0, "OS-EXT-SRV-ATTR:hostname": "cirros-test", "OS-EXT-SRV-ATTR:kernel_id": "", "OS-EXT-SRV-ATTR:ramdisk_id": "", "OS-EXT-SRV-ATTR:root_device_name": "/dev/vda", "OS-EXT-SRV-ATTR:user_data": null, "OS-EXT-STS:task_state": null, "OS-EXT-STS:vm_state": "stopped", "OS-EXT-STS:power_state": 4, "os-extended-volumes:volumes_attached": [{"id": "e6565b2e-6f99-45e8-9278-fd4b4b35a1ea", "delete_on_termination": false}], "host_status": "UP", "locked": false, "description": "test foo bar"}}`)
	err := json.Unmarshal(sejson, &dejson)
	if err != nil {
		t.Fatal(err)
	}

	sr := servers.GetResult{}
	sr.Body = dejson

	var s ExtendedServer
	err = sr.ExtractInto(&s)

	assert.NoError(err, "expect no error during extract")
	assert.Equal(s.Name, "cirros-tiny", "expect name to be 'cirros-tiny'")
	assert.Equal(s.Status, "", "expect status to be 'SHUTOFF'")
	assert.Equal(s.Description, "test foo bar", "expect description to be 'test foo bar'")
}
