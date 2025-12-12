package kvm

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirt "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/qemu"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
)

type Client struct {
	ctx        context.Context
	sshClient  *ssh.Client
	libvirtURI string
}

func NewClient(ctx context.Context, libvirtURI string, secret *corev1.Secret) (*Client, error) {
	u, err := url.Parse(libvirtURI)
	if err != nil {
		return nil, fmt.Errorf("invalid libvirt URI: %v", err)
	}

	// Expected URI format: qemu+ssh://user@host/system or just ssh://user@host
	// We extract host and user from the URI.
	host := u.Host
	user := u.User.Username()
	if user == "" {
		// Try to get user from secret if not in URI
		if secretUser, ok := secret.Data["username"]; ok {
			user = string(secretUser)
		} else {
			return nil, fmt.Errorf("username not found in URI or secret")
		}
	}

	authMethods := []ssh.AuthMethod{}
	if privateKey, ok := secret.Data["privateKey"]; ok {
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if password, ok := secret.Data["password"]; ok {
		authMethods = append(authMethods, ssh.Password(string(password)))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods provided in secret")
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// InsecureSkipTLSVerify logic handled by caller or ignored for now as we don't have known_hosts management easily
			// For this POC/implementation, we accept all keys if InsecureSkipTLSVerify is true (passed down? No, we don't have it here yet)
			// The CRD has the flag.
			return nil // TODO: Implement strict host checking if needed
		},
		Timeout: 10 * time.Second,
	}

	// Handle port if missing
	if !strings.Contains(host, ":") {
		host = host + ":22"
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial ssh: %v", err)
	}

	return &Client{
		ctx:        ctx,
		sshClient:  client,
		libvirtURI: libvirtURI,
	}, nil
}

func (c *Client) Close() error {
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}

func (c *Client) Verify() error {
	// We run a simple command to verify that the connection is working and we can talk to libvirt.
	// "virsh list --name" is lightweight and lists running domains.
	_, err := c.runCommand("virsh list --name")
	if err != nil {
		return fmt.Errorf("failed to verify connection to libvirt: %v", err)
	}
	return nil
}

func (c *Client) runCommand(cmd string) (string, error) {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	if err != nil {
		return "", fmt.Errorf("command %q failed: %v, stderr: %s", cmd, err, stderr.String())
	}
	return stdout.String(), nil
}

// Domain XML structs
type Domain struct {
	Name    string  `xml:"name"`
	UUID    string  `xml:"uuid"`
	Memory  int     `xml:"memory"`
	VCPU    int     `xml:"vcpu"`
	Devices Devices `xml:"devices"`
	OS      OS      `xml:"os"`
}

type OS struct {
	Type    OSType    `xml:"type"`
	BootDev []BootDev `xml:"boot>dev"`
}

type OSType struct {
	Arch    string `xml:"arch,attr"`
	Machine string `xml:"machine,attr"`
	Value   string `xml:",chardata"`
}

type BootDev struct {
	Dev string `xml:"dev,attr"`
}

type Devices struct {
	Disks      []Disk      `xml:"disk"`
	Interfaces []Interface `xml:"interface"`
}

type Disk struct {
	Device string `xml:"device,attr"`
	Driver Driver `xml:"driver"`
	Source Source `xml:"source"`
	Target Target `xml:"target"`
}

type Driver struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
}

type Source struct {
	File string `xml:"file,attr"`
}

type Target struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

type Interface struct {
	Type   string        `xml:"type,attr"`
	Mac    Mac           `xml:"mac"`
	Model  Model         `xml:"model"`
	Source NetworkSource `xml:"source"`
}

type Mac struct {
	Address string `xml:"address,attr"`
}

type Model struct {
	Type string `xml:"type,attr"`
}

type NetworkSource struct {
	Network string `xml:"network,attr"`
	Bridge  string `xml:"bridge,attr"`
}

func (c *Client) getDomainXML(vmName string) (*Domain, error) {
	out, err := c.runCommand(fmt.Sprintf("virsh dumpxml %s", vmName))
	if err != nil {
		return nil, err
	}

	var dom Domain
	if err := xml.Unmarshal([]byte(out), &dom); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain xml: %v", err)
	}
	return &dom, nil
}

func (c *Client) SanitizeVirtualMachineImport(vm *migration.VirtualMachineImport) error {
	vm.Status.ImportedVirtualMachineName = strings.ToLower(vm.Spec.VirtualMachineName)
	return nil
}

func (c *Client) ExportVirtualMachine(vm *migration.VirtualMachineImport) error {
	dom, err := c.getDomainXML(vm.Spec.VirtualMachineName)
	if err != nil {
		return err
	}

	tmpPath, err := os.MkdirTemp("/tmp", fmt.Sprintf("%s-%s-", vm.Name, vm.Namespace))
	if err != nil {
		return fmt.Errorf("error creating tmp dir: %v", err)
	}
	// Note: We don't defer removeAll here because we might want to keep it if something fails?
	// Actually, we should clean up. But the VMware client does defer removeAll.
	defer os.RemoveAll(tmpPath)

	for i, disk := range dom.Devices.Disks {
		if disk.Device != "disk" {
			continue
		}
		if disk.Source.File == "" {
			continue
		}

		// Local destination path
		rawDiskName := fmt.Sprintf("%s-disk-%d.img", vm.Name, i)
		destFile := filepath.Join(server.TempDir(), rawDiskName)

		logrus.Infof("Exporting disk %s to %s", disk.Source.File, destFile)

		// Use ssh cat | qemu-img convert
		// We need a new session for the pipe
		session, err := c.sshClient.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create ssh session: %v", err)
		}

		stdout, err := session.StdoutPipe()
		if err != nil {
			session.Close()
			return fmt.Errorf("failed to get stdout pipe: %v", err)
		}

		if err := session.Start(fmt.Sprintf("cat %s", disk.Source.File)); err != nil {
			session.Close()
			return fmt.Errorf("failed to start cat command: %v", err)
		}

		// Run qemu-img convert reading from stdin
		// XML has driver type.
		srcFormat := disk.Driver.Type
		if srcFormat == "" {
			srcFormat = "qcow2" // Default assumption
		}

		// Use shared qemu package
		if err := qemu.ConvertFromStdin(stdout, destFile, srcFormat); err != nil {
			session.Close()
			return fmt.Errorf("qemu convert failed: %v", err)
		}

		if err := session.Wait(); err != nil {
			return fmt.Errorf("ssh cat command failed: %v", err)
		}
		session.Close()

		// Update status
		busType := kubevirt.DiskBusVirtio
		if disk.Target.Bus == "sata" || disk.Target.Bus == "ide" {
			busType = kubevirt.DiskBusSATA
		} else if disk.Target.Bus == "scsi" {
			busType = kubevirt.DiskBusSCSI
		}

		vm.Status.DiskImportStatus = append(vm.Status.DiskImportStatus, migration.DiskInfo{
			Name:          rawDiskName,
			DiskSize:      0, // We could get this from qemu-img info on the result
			BusType:       busType,
			DiskLocalPath: server.TempDir(),
		})
	}

	return nil
}

func (c *Client) ShutdownGuest(vm *migration.VirtualMachineImport) error {
	_, err := c.runCommand(fmt.Sprintf("virsh shutdown %s", vm.Spec.VirtualMachineName))
	return err
}

func (c *Client) PowerOff(vm *migration.VirtualMachineImport) error {
	_, err := c.runCommand(fmt.Sprintf("virsh destroy %s", vm.Spec.VirtualMachineName))
	return err
}

func (c *Client) IsPowerOffSupported() bool {
	return true
}

func (c *Client) IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error) {
	out, err := c.runCommand(fmt.Sprintf("virsh domstate %s", vm.Spec.VirtualMachineName))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "shut off", nil
}

func (c *Client) GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error) {
	dom, err := c.getDomainXML(vm.Spec.VirtualMachineName)
	if err != nil {
		return nil, err
	}

	newVM := &kubevirt.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.Status.ImportedVirtualMachineName,
			Namespace: vm.Namespace,
		},
	}

	vmSpec := source.NewVirtualMachineSpec(source.VirtualMachineSpecConfig{
		Name: vm.Status.ImportedVirtualMachineName,
		Hardware: source.Hardware{
			NumCPU:   uint32(dom.VCPU),
			MemoryMB: int64(dom.Memory / 1024), // XML memory is usually in KiB
		},
	})

	// Network mapping
	var networkInfos []source.NetworkInfo
	for _, iface := range dom.Devices.Interfaces {
		model := migration.NetworkInterfaceModelVirtio
		if iface.Model.Type == "e1000" {
			model = migration.NetworkInterfaceModelE1000
		} else if iface.Model.Type == "e1000e" {
			model = migration.NetworkInterfaceModelE1000e
		}

		networkName := iface.Source.Network
		if networkName == "" {
			networkName = iface.Source.Bridge
		}

		networkInfos = append(networkInfos, source.NetworkInfo{
			NetworkName: networkName,
			MAC:         iface.Mac.Address,
			Model:       model,
		})
	}

	mappedNetwork := source.MapNetworks(networkInfos, vm.Spec.Mapping)
	networkConfig, interfaceConfig := source.GenerateNetworkInterfaceConfigs(mappedNetwork, vm.GetDefaultNetworkInterfaceModel())

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaceConfig
	newVM.Spec = *vmSpec

	return newVM, nil
}

func (c *Client) PreFlightChecks(vm *migration.VirtualMachineImport) error {
	// Check if VM exists
	_, err := c.runCommand(fmt.Sprintf("virsh dominfo %s", vm.Spec.VirtualMachineName))
	if err != nil {
		return fmt.Errorf("failed to find VM %s: %v", vm.Spec.VirtualMachineName, err)
	}
	return nil
}

func (c *Client) Cleanup(vm *migration.VirtualMachineImport) error {
	return source.RemoveTempImageFiles(vm.Status.DiskImportStatus)
}
