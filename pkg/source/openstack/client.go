package openstack

import (
	"context"

	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kubevirt "kubevirt.io/api/core/v1"
)

type Client struct {
}

func NewClient(ctx context.Context, endpoint string, dc string, secret *corev1.Secret) (*Client, error) {
	return nil, nil
}

func (c *Client) ExportVirtualMachine(vm *importjob.VirtualMachine) error {
	return nil
}

func (c *Client) PowerOffVirtualMachine(vm *importjob.VirtualMachine) error {
	return nil
}

func (c *Client) IsPoweredOff(vm *importjob.VirtualMachine) (bool, error) {
	return false, nil
}

func (c *Client) GenerateVirtualMachine(vm *importjob.VirtualMachine) (*kubevirt.VirtualMachine, error) {

	return nil, nil
}
