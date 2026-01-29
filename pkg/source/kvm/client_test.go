package kvm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	libvirtxml "libvirt.org/go/libvirtxml"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
)

func TestParseDomainXML(t *testing.T) {
	xmlData := `
<domain type='kvm' id='1'>
  <name>test-vm</name>
  <uuid>a75aca4b-42f6-4447-9262-4b9562d3d95c</uuid>
  <memory unit='KiB'>4194304</memory>
  <vcpu placement='static'>2</vcpu>
  <os>
    <type arch='x86_64'>hvm</type>
    <boot dev='hd'/>
  </os>
  <cpu mode='custom'>
    <model>IvyBridge-IBRS</model>
  </cpu>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/test-vm.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <interface type='network'>
      <mac address='52:54:00:6b:3c:58'/>
      <source network='default'/>
      <model type='virtio'/>
    </interface>
  </devices>
</domain>
`

	var dom libvirtxml.Domain
	err := dom.Unmarshal(xmlData)
	assert.NoError(t, err)

	assert.Equal(t, "test-vm", dom.Name)
	assert.Equal(t, "a75aca4b-42f6-4447-9262-4b9562d3d95c", dom.UUID)
	assert.Equal(t, uint(4194304), dom.Memory.Value)
	assert.Equal(t, uint(2), dom.VCPU.Value)
	assert.Len(t, dom.Devices.Disks, 1)
	assert.Equal(t, "/var/lib/libvirt/images/test-vm.qcow2", dom.Devices.Disks[0].Source.File.File)
	assert.Equal(t, "qcow2", dom.Devices.Disks[0].Driver.Type)
	assert.Len(t, dom.Devices.Interfaces, 1)
	assert.Equal(t, "52:54:00:6b:3c:58", dom.Devices.Interfaces[0].MAC.Address)
	assert.Equal(t, "default", dom.Devices.Interfaces[0].Source.Network.Network)
	assert.Equal(t, "IvyBridge-IBRS", dom.CPU.Model.Value)
}

func TestNewClient(t *testing.T) {
	ctx := context.TODO()
	secret := &corev1.Secret{
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	// Test with a dummy URI. Since we are using standard ssh.Dial,
	// checking valid URI parsing is still useful, even if Dial fails.
	_, err := NewClient(ctx, "ssh://user@localhost:2222", secret, migration.KVMSourceOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to dial endpoint")
}
