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
	"github.com/vmware/govmomi/vim25/mo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
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
	networkMapping := []migration.NetworkMapping{
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

	noNetworkMapping := []migration.NetworkMapping{}
	noMappedInfo := mapNetworkCards(networkInfo, noNetworkMapping)
	assert.Len(noMappedInfo, 0, "expected to find no item in the mapped networkinfo")
}
