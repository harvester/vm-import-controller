package openstack

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/blockstorage/extensions/volumeactions"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v2/volumes"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/imagedata"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
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

	code := t.Run()
	os.Exit(code)
}
func Test_NewClient(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")
	err = c.Verify()
	assert.NoError(err, "expect no error during verify of client")
}

func Test_checkOrGetUUID(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")
	err = c.Verify()
	assert.NoError(err, "expect no error during verify of client")
	vmName, ok := os.LookupEnv("VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	_, err = c.checkOrGetUUID(vmName)
	assert.NoError(err, "expected no error during checkOrGetUUID")
}

func Test_IsPoweredOff(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")
	err = c.Verify()
	assert.NoError(err, "expect no error during verify of client")
	vmName, ok := os.LookupEnv("VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	_, err = c.IsPoweredOff(vm)
	assert.NoError(err, "expected no error during check of power status")
}

func Test_PowerOffVirtualMachine(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")
	err = c.Verify()
	assert.NoError(err, "expect no error during verify of client")
	vmName, ok := os.LookupEnv("VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	err = c.PowerOffVirtualMachine(vm)
	assert.NoError(err, "expected no error during check of power status")
}

func Test_ExportVirtualMachine(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")
	err = c.Verify()
	assert.NoError(err, "expect no error during verify of client")
	vmName, ok := os.LookupEnv("VM_NAME")
	assert.True(ok, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	err = c.ExportVirtualMachine(vm)
	assert.NoError(err, "expected no error during exportvirtualmachines")
}

func Test_CreateImage(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")

	logrus.Info("attempting to create new image from volume")

	volID := "d9475b97-11a6-4711-bbb8-68271396e782"

	volume, err := volumes.Get(c.storageClient, volID).Extract()
	assert.NoError(err)
	volImage, err := volumeactions.UploadImage(c.storageClient, volume.ID, volumeactions.UploadImageOpts{
		ImageName:  "demo-from-test",
		DiskFormat: "qcow2",
	}).Extract()
	assert.NoError(err)
	t.Log(volImage)

	for {
		imgObj, err := images.Get(c.imageClient, volImage.ImageID).Extract()
		assert.NoError(err)
		assert.NotNil(imgObj)
		if imgObj.Status == "active" {
			break
		}
		time.Sleep(10 * time.Second)
	}

	contents, err := imagedata.Download(c.imageClient, volImage.ImageID).Extract()
	assert.NoError(err)

	imageContents, err := ioutil.ReadAll(contents)
	assert.NoError(err)
	tmpFile, err := ioutil.TempFile("/tmp", "contents")
	assert.NoError(err)
	_, err = tmpFile.Write(imageContents)
	assert.NoError(err)
	tmpFile.Close()

}

func Test_GenerateVirtualMachine(t *testing.T) {
	assert := require.New(t)
	vmName := os.Getenv("VM_NAME")
	assert.NotEmpty(vmName, "expected env variable VM_NAME to be set")
	vm := &importjob.VirtualMachine{
		Spec: importjob.VirtualMachineImportSpec{
			VirtualMachineName: vmName,
		},
	}
	_, err := c.GenerateVirtualMachine(vm)
	assert.NoError(err, "expected no error during GenerateVirtualMachine")

}
