package vmware

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/vim25/mo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var vcsimPort string

// setup mock vmware endpoint
func TestMain(t *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("error connecting to dockerd: %v", err)
	}

	runOpts := &dockertest.RunOptions{
		Name:       "vcsim",
		Repository: "vmware/vcsim",
		Tag:        "v0.29.0",
	}

	vcsimMock, err := pool.RunWithOptions(runOpts)

	if err != nil {
		log.Fatalf("error creating vcsim container: %v", err)
	}

	vcsimPort = vcsimMock.GetPort("8989/tcp")
	time.Sleep(30 * time.Second)
	go func() {
		server.NewServer(context.TODO())
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

	vm := &importjob.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: importjob.VirtualMachineImportSpec{
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

	vm := &importjob.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: importjob.VirtualMachineImportSpec{
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
	ctx := context.TODO()
	assert := require.New(t)
	govc_url := os.Getenv("GOVC_URL")
	assert.NotEmpty(govc_url, "expected govc_url to be set")
	govc_datacenter := os.Getenv("GOVC_DATACENTER")
	assert.NotEmpty(govc_datacenter, "expected govc_datacenter to be set")

	data := make(map[string]string)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	govc_username := os.Getenv("GOVC_USERNAME")
	assert.NotEmpty(govc_username, "expected govc_username to be set")
	data["username"] = govc_username

	govc_password := os.Getenv("GOVC_PASSWORD")
	assert.NotEmpty(govc_password, "expected govc_password to be set")
	data["password"] = govc_password
	secret.StringData = data

	vm_name := os.Getenv("VM_NAME")
	assert.NotEmpty(vm_name, "expected vm_name to be set")

	c, err := NewClient(ctx, govc_url, govc_datacenter, secret)
	assert.NoError(err, "expected no error during creation of client")
	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")

	vm := &importjob.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: importjob.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: vm_name,
		},
	}

	err = c.ExportVirtualMachine(vm)
	assert.NoError(err, "expected no error during vm export")
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

	vm := &importjob.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: importjob.VirtualMachineImportSpec{
			SourceCluster:      corev1.ObjectReference{},
			VirtualMachineName: "DC0_H0_VM0",
			Mapping: []importjob.NetworkMapping{
				{
					SourceNetwork:      "DVSwitch: fea97929-4b2d-5972-b146-930c6d0b4014",
					DestinationNetwork: "default/vlan",
				},
			},
		},
	}

	newVM, err := c.GenerateVirtualMachine(vm)
	assert.NoError(err, "expected no error during vm export")
	assert.Len(newVM.Spec.Template.Spec.Networks, 1, "should have found the default pod network")
	assert.Len(newVM.Spec.Template.Spec.Domain.Devices.Interfaces, 1, "should have found a network map")
	assert.Equal(newVM.Spec.Template.Spec.Domain.Memory.Guest.String(), "32M", "expected VM to have 32M memory")
	assert.NotEmpty(newVM.Spec.Template.Spec.Domain.Resources.Limits, "expect to find resource requests to be present")

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
	assert.NoError(err, "expected no error during vm lookup")

	var o mo.VirtualMachine

	err = vmObj.Properties(c.ctx, vmObj.Reference(), []string{}, &o)
	assert.NoError(err, "expected no error looking up vmObj properties")

	networkInfo := identifyNetworkCards(o.Config.Hardware.Device)
	assert.Len(networkInfo, 1, "expected to find only 1 item in the networkInfo")
	networkMapping := []importjob.NetworkMapping{
		{
			SourceNetwork:      "dummyNetwork",
			DestinationNetwork: "harvester1",
		},
		{
			SourceNetwork:      "DVSwitch: fea97929-4b2d-5972-b146-930c6d0b4014",
			DestinationNetwork: "pod-network",
		},
	}

	mappedInfo := mapNetworkCards(networkInfo, networkMapping)
	assert.Len(mappedInfo, 1, "expected to find only 1 item in the mapped networkinfo")

	noNetworkMapping := []importjob.NetworkMapping{}
	noMappedInfo := mapNetworkCards(networkInfo, noNetworkMapping)
	assert.Len(noMappedInfo, 0, "expected to find no item in the mapped networkinfo")
}
