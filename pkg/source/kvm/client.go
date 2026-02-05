package kvm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirt "kubevirt.io/api/core/v1"
	libvirtxml "libvirt.org/go/libvirtxml"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/qemu"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
)

type Client struct {
	ctx       context.Context
	options   migration.KVMSourceOptions
	sshClient *ssh.Client
}

func NewClient(ctx context.Context, endpoint string, secret *corev1.Secret, options migration.KVMSourceOptions) (*Client, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error parsing endpoint URL: %w", err)
	}

	host := endpointURL.Host
	user := endpointURL.User.Username()
	if user == "" {
		if secretUser, ok := secret.Data["username"]; ok {
			user = string(secretUser)
		} else {
			return nil, fmt.Errorf("username not found in endpoint URL or secret")
		}
	}

	authMethods := []ssh.AuthMethod{}
	if privateKey, ok := secret.Data["privateKey"]; ok {
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if password, ok := secret.Data["password"]; ok {
		authMethods = append(authMethods, ssh.Password(string(password)))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods provided in secret")
	}

	sshClientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // nolint:gosec
		Timeout:         options.GetSSHTimeout(),
	}

	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s:22", host)
	}

	logrus.WithFields(logrus.Fields{
		"host":    host,
		"user":    user,
		"timeout": options.GetSSHTimeout().String(),
	}).Info("Dialing endpoint ...")
	sshClient, err := ssh.Dial("tcp", host, sshClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial endpoint: %w", err)
	}

	return &Client{
		ctx:       ctx,
		options:   options,
		sshClient: sshClient,
	}, nil
}

func (c *Client) Close() error {
	if c.sshClient != nil {
		return c.sshClient.Close()
	}
	return nil
}

func (c *Client) Verify() error {
	// We run a simple command to verify that the connection is working, and we can talk to libvirt.
	// "virsh list --name" is lightweight and lists running domains.
	_, err := c.runCommand([]string{"list", "--name"})
	if err != nil {
		return fmt.Errorf("failed to verify connection to %s: %w", c.sshClient.RemoteAddr().String(), err)
	}
	logrus.Infof("Connection verified to %s", c.sshClient.RemoteAddr().String())
	return nil
}

func (c *Client) runCommand(args []string) (string, error) {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	uri := c.options.GetVirshConnectionURI()
	if uri != "" {
		args = append([]string{"-c", uri}, args...)
	}

	cmd := exec.Command("virsh", args...)
	cmdLine := cmd.String()

	err = session.Run(cmdLine)
	if err != nil {
		return "", fmt.Errorf("command %q failed: %v, stderr: %s", cmdLine, err, stderr.String())
	}

	return stdout.String(), nil
}

func (c *Client) getDomainXML(vmName string) (*libvirtxml.Domain, error) {
	out, err := c.runCommand([]string{"dumpxml", vmName})
	if err != nil {
		return nil, err
	}

	var dom libvirtxml.Domain
	if err := dom.Unmarshal(out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain xml: %w", err)
	}
	return &dom, nil
}

func (c *Client) SanitizeVirtualMachineImport(vm *migration.VirtualMachineImport) error {
	vm.Status.ImportedVirtualMachineName = strings.Split(strings.ToLower(vm.Spec.VirtualMachineName), ".")[0]
	return nil
}

func (c *Client) ExportVirtualMachine(vm *migration.VirtualMachineImport) error {
	dom, err := c.getDomainXML(vm.Spec.VirtualMachineName)
	if err != nil {
		return err
	}

	sftpClient, err := sftp.NewClient(c.sshClient)
	if err != nil {
		return fmt.Errorf("failed to create sftp client: %w", err)
	}
	defer sftpClient.Close()

	if dom.Devices == nil {
		return fmt.Errorf("no devices found in domain XML")
	}

	for i, disk := range dom.Devices.Disks {
		if disk.Device != "disk" {
			continue
		}
		var sourceFile string
		if disk.Source != nil {
			if disk.Source.File != nil {
				sourceFile = disk.Source.File.File
			} else if disk.Source.Block != nil {
				sourceFile = disk.Source.Block.Dev
			}
		}

		if sourceFile == "" {
			continue
		}

		// Create a temporary file to store the downloaded disk
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("%s-disk-%d-", vm.Name, i))
		if err != nil {
			return fmt.Errorf("failed to create temporary file for download: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		logrus.Infof("Downloading disk %s to %s", sourceFile, tmpFile.Name())

		// Open the remote file
		remoteFile, err := sftpClient.Open(sourceFile)
		if err != nil {
			return fmt.Errorf("failed to open remote file %s: %w", sourceFile, err)
		}
		defer remoteFile.Close()

		// Copy the remote file to the temporary local file
		if _, err := io.Copy(tmpFile, remoteFile); err != nil {
			return fmt.Errorf("failed to download remote file: %w", err)
		}
		tmpFile.Close() // Close the file so qemu-img can access it

		// Local destination path for the converted RAW image
		rawDiskName := source.GenerateRawImageFileName(vm.Name, i)
		destFile := filepath.Join(server.TempDir(), rawDiskName)

		logrus.Infof("Converting downloaded disk %s to %s", tmpFile.Name(), destFile)

		// Use qemu-img convert on the local, downloaded file
		format := "qcow2" // Default assumption
		if disk.Driver != nil && disk.Driver.Type != "" {
			format = disk.Driver.Type
		}

		if err := qemu.ConvertToRAW(tmpFile.Name(), destFile, format); err != nil {
			return fmt.Errorf("qemu convert failed: %w", err)
		}

		// Update status
		busType := kubevirt.DiskBusVirtio
		if disk.Target != nil {
			switch disk.Target.Bus {
			case "sata", "ide":
				busType = kubevirt.DiskBusSATA
			case "scsi":
				busType = kubevirt.DiskBusSCSI
			}
		}

		// Get the size of the converted image
		destFileInfo, err := os.Stat(destFile)
		if err != nil {
			return fmt.Errorf("failed to get stats for converted disk: %w", err)
		}

		vm.Status.DiskImportStatus = append(vm.Status.DiskImportStatus, migration.DiskInfo{
			Name:          rawDiskName,
			DiskSize:      destFileInfo.Size(),
			BusType:       busType,
			DiskLocalPath: server.TempDir(),
		})
	}

	return nil
}

func (c *Client) ShutdownGuest(vm *migration.VirtualMachineImport) error {
	powerOff, err := c.IsPoweredOff(vm)
	if err != nil {
		return err
	}
	if powerOff {
		logrus.Infof("VM %s is already powered off, skipping shutdown.", vm.Spec.VirtualMachineName)
		return nil
	}
	_, err = c.runCommand([]string{"shutdown", vm.Spec.VirtualMachineName})
	return err
}

func (c *Client) PowerOff(vm *migration.VirtualMachineImport) error {
	_, err := c.runCommand([]string{"destroy", vm.Spec.VirtualMachineName})
	return err
}

func (c *Client) IsPowerOffSupported() bool {
	return true
}

func (c *Client) IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error) {
	out, err := c.runCommand([]string{"domstate", vm.Spec.VirtualMachineName})
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

	cpuModel := ""
	if dom.CPU != nil && dom.CPU.Model != nil {
		cpuModel = dom.CPU.Model.Value
	}

	// Firmware settings
	fw := source.NewFirmware(false, false, false)
	if dom.OS != nil && dom.OS.Loader != nil {
		fw.UEFI = true // The presence of a loader usually indicates UEFI
	}
	if dom.Devices != nil && len(dom.Devices.TPMs) > 0 {
		fw.TPM = true
	}

	vmSpec := source.NewVirtualMachineSpec(source.VirtualMachineSpecConfig{
		Name: vm.Status.ImportedVirtualMachineName,
		Hardware: source.Hardware{
			NumCPU:   uint32(dom.VCPU.Value),         // nolint:gosec
			MemoryMB: int64(dom.Memory.Value / 1024), // nolint:gosec // XML memory is usually in KiB
			CPUModel: cpuModel,
		},
	})

	// Network mapping
	var networkInfos []source.NetworkInfo
	if dom.Devices != nil {
		for _, iface := range dom.Devices.Interfaces {
			model := migration.NetworkInterfaceModelVirtio
			if iface.Model != nil {
				switch iface.Model.Type {
				case "e1000":
					model = migration.NetworkInterfaceModelE1000
				case "e1000e":
					model = migration.NetworkInterfaceModelE1000e
				}
			}

			networkName := ""
			if iface.Source != nil {
				if iface.Source.Network != nil {
					networkName = iface.Source.Network.Network
				} else if iface.Source.Bridge != nil {
					networkName = iface.Source.Bridge.Bridge
				}
			}

			macAddr := ""
			if iface.MAC != nil {
				macAddr = iface.MAC.Address
			}

			networkInfos = append(networkInfos, source.NetworkInfo{
				NetworkName: networkName,
				MAC:         macAddr,
				Model:       model,
			})
		}
	}

	mappedNetwork := source.MapNetworks(networkInfos, vm.Spec.Mapping)
	networkConfig, interfaceConfig := source.GenerateNetworkInterfaceConfigs(mappedNetwork, vm.GetDefaultNetworkInterfaceModel())

	// Apply firmware settings
	source.ApplyFirmwareSettings(vmSpec, fw)

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaceConfig
	newVM.Spec = *vmSpec

	return newVM, nil
}

func (c *Client) PreFlightChecks(vm *migration.VirtualMachineImport) error {
	// Check if VM exists
	_, err := c.runCommand([]string{"dominfo", vm.Spec.VirtualMachineName})
	if err != nil {
		return fmt.Errorf("failed to find VM %s: %w", vm.Spec.VirtualMachineName, err)
	}
	return nil
}

func (c *Client) Cleanup(vm *migration.VirtualMachineImport) error {
	return source.RemoveTempImageFiles(vm.Status.DiskImportStatus)
}
