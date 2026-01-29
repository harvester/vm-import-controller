package ova

import (
	"bufio"
	"context"
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/ovf/importer"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/qemu"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
	"github.com/harvester/vm-import-controller/pkg/util"
)

// References:
// - https://www.dmtf.org/standards/ovf
// - https://www.dmtf.org/sites/default/files/standards/documents/DSP0243_2.1.1.pdf
// - https://www.dmtf.org/sites/default/files/standards/documents/DSP0004V2.3_final.pdf
// - https://github.com/vmware/open-vmdk

var unitToBytesMap = map[string]int64{
	"kb":        1024, // 2^10
	"kilobytes": 1024,
	"mb":        1024 * 1024, // 2^20
	"megabytes": 1024 * 1024,
	"gb":        1024 * 1024 * 1024, // 2^30
	"gigabytes": 1024 * 1024 * 1024,
	"tb":        1024 * 1024 * 1024 * 1024, // 2^40
	"terabytes": 1024 * 1024 * 1024 * 1024,
}

type Client struct {
	ctx        context.Context
	url        string
	secret     *corev1.Secret
	httpClient *http.Client
	options    migration.OvaSourceOptions
	workingDir string
}

func NewClient(ctx context.Context, url string, secret *corev1.Secret, options migration.OvaSourceOptions) (*Client, error) {
	httpClient, err := newHttpClient(secret, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	return &Client{
		ctx:        ctx,
		url:        url,
		secret:     secret,
		httpClient: httpClient,
		options:    options,
		workingDir: server.TempDir(),
	}, nil
}

func (c *Client) Verify() error {
	// Verify if the URL is valid, has the correct scheme and exists.
	parsedUrl, err := url.ParseRequestURI(c.url)
	if err != nil {
		return fmt.Errorf("error parsing URL: %w", err)
	}

	switch parsedUrl.Scheme {
	case "http", "https":
		req, err := newHttpRequest("HEAD", c.url, c.secret)
		if err != nil {
			return err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make HEAD request: %w", err)
		}
		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("failed %s request (code=%d): %s", req.Method, resp.StatusCode, resp.Status)
		}
	default:
		return fmt.Errorf("unsupported URL scheme %q: must be 'http' or 'https'", parsedUrl.Scheme)
	}

	return nil
}

// PreFlightChecks is required by the `VirtualMachineOperations` interface.
func (c *Client) PreFlightChecks(_ *migration.VirtualMachineImport) (err error) {
	return nil
}

// SanitizeVirtualMachineImport is required by the `VirtualMachineOperations` interface.
func (c *Client) SanitizeVirtualMachineImport(vmi *migration.VirtualMachineImport) error {
	// Note, VMware allows upper case characters in virtual machine names,
	// so we need to convert them to lower case to be RFC 1123 compliant.
	vmi.Status.ImportedVirtualMachineName = strings.ToLower(vmi.Spec.VirtualMachineName)

	return nil
}

// ExportVirtualMachine is required by the `VirtualMachineOperations` interface.
// The following steps are performed:
// - Download the OVA file to /tmp.
// - Read the OVF envelope from the OVA file.
// - Create a `DiskInfo` object for each disk described in the OVF.
// - Extract each VMDK file from the OVA file, verify its checksum and convert it to RAW format.
// - Append the `DiskInfo` objects to the `DiskImportStatus` field of the `VirtualMachineImport` object.
func (c *Client) ExportVirtualMachine(vmi *migration.VirtualMachineImport) error {
	tempArchivePath := c.generateArchivePath(vmi)

	err := c.downloadArchive(tempArchivePath)
	if err != nil {
		return err
	}

	e, err := readEnvelope(tempArchivePath)
	if err != nil {
		return fmt.Errorf("failed to read envelope: %w", err)
	}

	_, _, _, dis := parseEnvelope(e, vmi.GetDefaultNetworkInterfaceModel(), vmi.GetDefaultDiskBusType())
	logrus.WithFields(util.FieldsToJSON(logrus.Fields{
		"name":      vmi.Name,
		"namespace": vmi.Namespace,
		"diskInfos": dis,
	}, []string{"diskInfos"})).Info("Parsed disk information from OVF envelope")

	for _, di := range dis {
		tempImagePath := c.generateImagePath(vmi, di)

		err = c.extractAndConvertVMDKToRAW(tempArchivePath, di.Name, tempImagePath, true)
		if err != nil {
			return err
		}

		// Patch several fields.
		di.Name = filepath.Base(tempImagePath)
		di.DiskLocalPath = filepath.Dir(tempImagePath)

		vmi.Status.DiskImportStatus = append(vmi.Status.DiskImportStatus, di)
	}

	return nil
}

// GenerateVirtualMachine is required by the `VirtualMachineOperations` interface.
func (c *Client) GenerateVirtualMachine(vmi *migration.VirtualMachineImport) (*kubevirtv1.VirtualMachine, error) {
	tempArchivePath := c.generateArchivePath(vmi)

	e, err := readEnvelope(tempArchivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read envelope: %w", err)
	}

	fw, hw, nis, dis := parseEnvelope(e, vmi.GetDefaultNetworkInterfaceModel(), vmi.GetDefaultDiskBusType())
	logrus.WithFields(util.FieldsToJSON(logrus.Fields{
		"name":         vmi.Name,
		"namespace":    vmi.Namespace,
		"firmware":     fw,
		"hardware":     hw,
		"networkInfos": nis,
		"diskInfos":    dis,
	}, []string{"firmware", "hardware", "networkInfos", "diskInfos"})).Info("Parsed configuration from OVF envelope")

	newVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmi.Status.ImportedVirtualMachineName,
			Namespace: vmi.Namespace,
		},
	}

	vmSpec := source.NewVirtualMachineSpec(source.VirtualMachineSpecConfig{
		Name: vmi.Status.ImportedVirtualMachineName,
		Hardware: source.Hardware{
			NumCPU:            hw.NumCPU,
			NumCoresPerSocket: 1, // OVF does not provide cores per socket information.
			MemoryMB:          hw.MemoryMB,
		},
	})

	mappedNetwork := source.MapNetworks(nis, vmi.Spec.Mapping)
	networkConfig, interfaceConfig := source.GenerateNetworkInterfaceConfigs(mappedNetwork, vmi.GetDefaultNetworkInterfaceModel())

	// Setup BIOS/EFI, SecureBoot and TPM settings.
	source.ApplyFirmwareSettings(vmSpec, fw)

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaceConfig
	newVM.Spec = *vmSpec

	return newVM, nil
}

// ShutdownGuest is required by the `VirtualMachineOperations` interface.
func (c *Client) ShutdownGuest(_ *migration.VirtualMachineImport) error {
	// Nothing to do here.
	return nil
}

// PowerOff is required by the `VirtualMachineOperations` interface.
func (c *Client) PowerOff(_ *migration.VirtualMachineImport) error {
	// Not implemented as OVA does not support guest OS operations.
	return nil
}

// IsPowerOffSupported is required by the `VirtualMachineOperations` interface.
func (c *Client) IsPowerOffSupported() bool {
	// Powering off the VM is not supported.
	return false
}

// IsPoweredOff is required by the `VirtualMachineOperations` interface.
func (c *Client) IsPoweredOff(_ *migration.VirtualMachineImport) (bool, error) {
	// The VM is always considered powered off.
	return true, nil
}

func (c *Client) Cleanup(vmi *migration.VirtualMachineImport) error {
	// - Do not abort the cleanup process on the first error to ensure all
	//   resources are cleaned up.
	// - Aggregate all errors that might occur during the cleanup process.
	var errs []error

	err := os.Remove(c.generateArchivePath(vmi))
	if err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("failed to remove downloaded OVA archive: %w", err))
	}

	err = source.RemoveTempImageFiles(vmi.Status.DiskImportStatus)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// downloadArchive downloads the OVA file to /tmp.
func (c *Client) downloadArchive(dstPath string) error {
	logrus.WithFields(logrus.Fields{
		"dstPath": dstPath,
	}).Info("Downloading OVA archive ...")

	req, err := newHttpRequest("GET", c.url, c.secret)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make GET request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed %s request (code=%d): %s", req.Method, resp.StatusCode, resp.Status)
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination archive file %q: %w", dstPath, err)
	}
	defer dst.Close() //nolint:errcheck

	_, err = io.Copy(dst, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write archive file %q: %w", dstPath, err)
	}

	return nil
}

// ReadManifest converts an ovf manifest to a map of file name -> Checksum.
// This is a hardened version of the `library.ReadManifest` function that can
// only parse `ALGO(<FILENAME>)= <CHECKSUM>` and fails for
// `ALGO(<FILENAME>) = <CHECKSUM>`.
// See https://github.com/vmware/govmomi/issues/3893
func readManifest(r io.Reader) (map[string]*library.Checksum, error) {
	csums := make(map[string]*library.Checksum)
	regex := regexp.MustCompile(`^(\w+)\(([^)]+)\)\s*=\s*([a-fA-F0-9]+)$`)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		matches := regex.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue
		}

		sum := &library.Checksum{
			Algorithm: strings.TrimSpace(matches[1]),
			Checksum:  strings.TrimSpace(matches[3]),
		}

		csums[matches[2]] = sum
	}

	return csums, scanner.Err()
}

// readEnvelope reads the OVF envelope from the OVA archive.
func readEnvelope(archivePath string) (*ovf.Envelope, error) {
	opener := importer.Opener{}
	archive := importer.TapeArchive{Path: archivePath, Opener: opener}

	o, err := importer.ReadOvf("*.ovf", &archive)
	if err != nil {
		return nil, fmt.Errorf("failed to read OVF from archive %q: %w", archivePath, err)
	}

	e, err := importer.ReadEnvelope(o)
	if err != nil {
		return nil, fmt.Errorf("failed to read envelope from archive %q: %w", archivePath, err)
	}

	return e, nil
}

// extractAndConvertVMDKToRAW extracts the VMDK file from the OVA archive,
// verifies its checksum and converts it to RAW format.
func (c *Client) extractAndConvertVMDKToRAW(archivePath, name, dstPath string, convert bool) error {
	opener := importer.Opener{}
	archive := importer.TapeArchive{Path: archivePath, Opener: opener}

	logrus.WithFields(logrus.Fields{
		"archivePath": archivePath,
		"name":        name,
		"dstPath":     dstPath,
	}).Info("Extracting VMDK from OVA archive and convert it to RAW ...")

	archiveFile, _, err := archive.Open(name)
	if err != nil {
		return fmt.Errorf("failed to open VMDK %q from %q: %w", name, archivePath, err)
	}

	defer archiveFile.Close() //nolint:errcheck

	mfFile, _, err := archive.Open("*.mf")
	if err != nil {
		return fmt.Errorf("failed to open manifest: %w", err)
	}

	defer mfFile.Close() //nolint:errcheck

	mf, err := readManifest(mfFile)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	csum, ok := mf[name]
	if !ok {
		return fmt.Errorf("failed to find entry for %q in manifest", name)
	}

	var hashFile hash.Hash
	switch strings.ToLower(csum.Algorithm) {
	case "sha1":
		hashFile = sha1.New() // nolint:gosec
	case "sha256":
		hashFile = sha256.New()
	default:
		return fmt.Errorf("unsupported checksum algorithm %q in manifest", csum.Algorithm)
	}

	vmdkFile, err := os.Create(filepath.Join(c.workingDir, name))
	if err != nil {
		return fmt.Errorf("failed to create VMDK file: %w", err)
	}

	defer func() {
		_ = vmdkFile.Close()
		_ = os.Remove(vmdkFile.Name())
	}()

	mwFile := io.MultiWriter(hashFile, vmdkFile)

	if _, err := io.Copy(mwFile, archiveFile); err != nil {
		return fmt.Errorf("failed to write VMDK file %q: %w", name, err)
	}

	checksum := hex.EncodeToString(hashFile.Sum(nil))
	if checksum != mf[name].Checksum {
		return fmt.Errorf("checksum mismatch for VMDK file %q: expected %q, got %q", name, mf[name].Checksum, checksum)
	}

	if convert {
		err = qemu.ConvertToRAW(vmdkFile.Name(), dstPath, "vmdk")
		if err != nil {
			return fmt.Errorf("failed to convert VMDK file %q to RAW %q: %w", vmdkFile.Name(), dstPath, err)
		}
	}

	return nil
}

func (c *Client) generateArchivePath(vmi *migration.VirtualMachineImport) string {
	return fmt.Sprintf("%s.ova", filepath.Join(c.workingDir, vmi.Status.ImportedVirtualMachineName))
}

func generateImageName(vmi *migration.VirtualMachineImport, di migration.DiskInfo) string {
	return fmt.Sprintf("%s-%s.img", vmi.Status.ImportedVirtualMachineName, util.BaseName(di.Name))
}

func (c *Client) generateImagePath(vmi *migration.VirtualMachineImport, di migration.DiskInfo) string {
	return filepath.Join(c.workingDir, generateImageName(vmi, di))
}

// parseCapacity parses the capacity of a disk from its OVF description and returns it in bytes.
func parseCapacity(disk ovf.VirtualDiskDesc) int64 {
	b, _ := strconv.ParseInt(disk.Capacity, 10, 64)

	if disk.CapacityAllocationUnits == nil {
		return b
	}

	// https://www.dmtf.org/sites/default/files/standards/documents/DSP0004_2.7.0.pdf - Page 164
	c := strings.Fields(*disk.CapacityAllocationUnits)
	if len(c) == 3 && c[0] == "byte" && c[1] == "*" { // e.g. "byte * 2^20"
		p := strings.Split(c[2], "^")
		x, _ := strconv.ParseInt(p[0], 10, 64)

		if len(p) == 2 {
			y, _ := strconv.ParseUint(p[1], 10, 64)
			b *= int64(math.Pow(float64(x), float64(y)))
		} else {
			b *= x
		}
	}
	if len(c) == 1 { // e.g., "MB"
		unit := strings.ToLower(c[0])
		if multiplier, ok := unitToBytesMap[unit]; ok {
			b *= multiplier
		}
	}

	return b
}

func parseVirtualQuantity(rasd ovf.ResourceAllocationSettingData) int64 {
	b := int64(*rasd.VirtualQuantity) // nolint:gosec

	if rasd.AllocationUnits == nil {
		return b
	}

	// https://www.dmtf.org/sites/default/files/standards/documents/DSP0004_2.7.0.pdf - Page 164
	c := strings.Fields(*rasd.AllocationUnits)
	if len(c) == 3 && c[0] == "byte" && c[1] == "*" { // e.g. "byte * 2^20"
		p := strings.Split(c[2], "^")
		x, _ := strconv.ParseInt(p[0], 10, 32)

		if len(p) == 2 {
			y, _ := strconv.ParseInt(p[1], 10, 32)
			b *= int64(math.Pow(float64(x), float64(y)))
		} else {
			b *= x
		}
	}
	if len(c) == 1 { // e.g., "MegaBytes"
		unit := strings.ToLower(c[0])
		if multiplier, ok := unitToBytesMap[unit]; ok {
			b *= multiplier
		}
	}

	return b
}

func findResourceAllocationSettingData(vhs *ovf.VirtualHardwareSection, instanceID string) *ovf.ResourceAllocationSettingData {
	for _, item := range vhs.Item {
		if item.InstanceID == instanceID {
			return &item
		}
	}
	return nil
}

func resolveReference(e *ovf.Envelope, id string) *ovf.File {
	for _, ref := range e.References {
		if ref.ID == id {
			return &ref
		}
	}
	return nil
}

// detectDiskBusType detects the disk bus type from the given controller ResourceAllocationSettingData.
func detectDiskBusType(rasd *ovf.ResourceAllocationSettingData) (kubevirtv1.DiskBus, bool) {
	var busType kubevirtv1.DiskBus
	// ResourceType | ResourceSubType
	// -------------|-----------------
	// 5            | PIIX4
	// 6	        | lsilogicsas
	// 6	        | lsilogic
	// 6            | VirtualSCSI
	// 20	        | vmware.usb.ehci
	// 20	        | vmware.nvme.controller
	// 20	        | vmware.sata.ahci
	// 20	        | vmware.usb.ehci
	// 20	        | AHCI
	// 23           | vmware.usb.xhci
	if rasd.ResourceSubType != nil {
		rst := strings.ToLower(*rasd.ResourceSubType)
		switch {
		case strings.Contains(rst, "scsi"), strings.Contains(rst, "buslogic"), strings.Contains(rst, "lsilogic"):
			busType = kubevirtv1.DiskBusSCSI
		case strings.Contains(rst, "ahci"), strings.Contains(rst, "sata"), strings.Contains(rst, "ide"):
			busType = kubevirtv1.DiskBusSATA
		case rst == "virtio", strings.Contains(rst, "nvme"):
			busType = kubevirtv1.DiskBusVirtio
		case strings.Contains(rst, "usb"):
			busType = kubevirtv1.DiskBusUSB
		}
	} else {
		if rasd.ResourceType != nil {
			switch *rasd.ResourceType {
			case ovf.IdeController:
				busType = kubevirtv1.DiskBusSATA
			case ovf.ParallelScsiHba:
				busType = kubevirtv1.DiskBusSCSI
			case ovf.OtherStorage:
				busType = kubevirtv1.DiskBusVirtio
			case ovf.UsbController:
				busType = kubevirtv1.DiskBusVirtio
			}
		}
	}
	return busType, busType != ""
}

// parseEnvelope retrieves the firmware, virtual hardware and network settings from the OVF envelope.
func parseEnvelope(e *ovf.Envelope, defaultInterfaceModel string, defaultDiskBusType kubevirtv1.DiskBus) (*source.Firmware, *source.Hardware, []source.NetworkInfo, []migration.DiskInfo) {
	fw := source.NewFirmware(false, false, false)
	hw := source.NewHardware(0, 0, 0, "")
	nis := make([]source.NetworkInfo, 0)
	dis := make([]migration.DiskInfo, 0)

	if e.VirtualSystem != nil {
		disks := ptr.Deref(e.Disk, ovf.DiskSection{Disks: []ovf.VirtualDiskDesc{}}).Disks

		for _, vh := range e.VirtualSystem.VirtualHardware {
			// OVF v1.0 - <Item>
			for _, item := range vh.Item {
				// References:
				// - https://github.com/vmware/govmomi/blob/7b6d4590646626c96685e17eb97e585a2ab43858/simulator/ovf_manager.go#L235
				// - https://opennodecloud.com/howto/2013/12/25/howto-ON-ovf-reference.html
				if item.ResourceType != nil {
					switch *item.ResourceType {
					case ovf.Processor: // Number of Virtual CPUs
						hw.NumCPU = uint32(*item.VirtualQuantity) // nolint:gosec
					case ovf.Memory:
						bytes := parseVirtualQuantity(item)
						hw.MemoryMB = bytes / (1024 * 1024)
					case ovf.EthernetAdapter:
						macAddress := ""
						if ptr.Deref(item.AutomaticAllocation, false) {
							if item.Address != nil {
								macAddress = *item.Address
							}
						}

						model := defaultInterfaceModel
						if item.ResourceSubType != nil {
							switch strings.ToLower(*item.ResourceSubType) {
							case "rtl8139":
								model = migration.NetworkInterfaceModelRtl8139
							case "e1000":
								model = migration.NetworkInterfaceModelE1000
							case "e1000e":
								model = migration.NetworkInterfaceModelE1000e
							case "pcnet32":
								model = migration.NetworkInterfaceModelPcnet
							case "vmxnet", "vmxnet2", "vmxnet3", "virtio":
								model = migration.NetworkInterfaceModelVirtio
							}
						}

						nis = append(nis, source.NetworkInfo{
							NetworkName: item.Connection[0],
							MAC:         macAddress,
							Model:       model,
						})
					case ovf.DiskDrive:
						diskId := filepath.Base(item.HostResource[0])
						for _, disk := range disks {
							if disk.DiskID == diskId {
								ref := resolveReference(e, *disk.FileRef)
								if ref != nil {
									busType := defaultDiskBusType

									if item.Parent != nil {
										parentItem := findResourceAllocationSettingData(&vh, *item.Parent)
										if parentItem != nil {
											if detectedBusType, ok := detectDiskBusType(parentItem); ok {
												busType = detectedBusType
											}
										}
									}

									dis = append(dis, migration.DiskInfo{
										Name:     ref.Href,
										DiskSize: parseCapacity(disk),
										BusType:  busType,
									})
								}
							}
						}
					}
				}

				// https://techdocs.broadcom.com/us/en/vmware-cis/desktop-hypervisors/workstation-pro/17-0/using-vmware-workstation-pro/configuring-and-managing-virtual-machines/export-a-virtual-machine-with-vtpm-device-to-ovf-format.html
				if item.ResourceSubType != nil {
					switch *item.ResourceSubType { // revive:disable:unnecessary-stmt
					case "vmware.vtpm":
						fw.TPM = true
					}
				}

				for _, cfg := range vh.Config {
					switch cfg.Key {
					case "bootOptions.efiSecureBootEnabled":
						fw.SecureBoot, _ = strconv.ParseBool(cfg.Value)
					case "firmware":
						if strings.ToLower(cfg.Value) == string(types.GuestOsDescriptorFirmwareTypeEfi) {
							fw.UEFI = true
						}
					// These configuration options are not confirmed (but found on
					// the web), so they should be used with caution.
					case "tpm.present", "tpm20.enabled":
						fw.TPM, _ = strconv.ParseBool(cfg.Value)
					}
				}
			}

			// OVF v2.0 - <StorageItem>
			// https://github.com/vmware/govmomi/issues/1606
			for _, item := range vh.StorageItem {
				if item.ResourceType != nil {
					switch *item.ResourceType {
					case ovf.DiskDrive:
						diskId := filepath.Base(item.HostResource[0])
						for _, disk := range disks {
							if disk.DiskID == diskId {
								ref := resolveReference(e, *disk.FileRef)
								if ref != nil {
									busType := defaultDiskBusType

									if item.Parent != nil {
										parentItem := findResourceAllocationSettingData(&vh, *item.Parent)
										if parentItem != nil {
											if detectedBusType, ok := detectDiskBusType(parentItem); ok {
												busType = detectedBusType
											}
										}
									}

									dis = append(dis, migration.DiskInfo{
										Name:     ref.Href,
										DiskSize: parseCapacity(disk),
										BusType:  busType,
									})
								}
							}
						}
					}
				}
			}

			// OVF v2.0 - <EthernetPortItem>
			// This is not yet supported by govmomi.
		}
	}

	return fw, hw, nis, dis
}

// newHttpRequest creates a new HTTP request with optional basic authentication.
func newHttpRequest(method, url string, secret *corev1.Secret) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	if secret != nil {
		username, usernameOk := secret.Data["username"]
		password, passwordOk := secret.Data["password"]
		if usernameOk && passwordOk {
			logrus.Info("Use credentials from the secret for basic authentication of HTTP requests")
			req.SetBasicAuth(string(username), string(password))
		}
	}

	return req, nil
}

// newHttpClient creates a new HTTP client with optional TLS configuration.
// The timeout is set based on the provided options.
func newHttpClient(secret *corev1.Secret, options migration.OvaSourceOptions) (*http.Client, error) {
	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	if secret != nil {
		pemBytes, ok := secret.Data["ca.crt"]
		if ok {
			logrus.Info("Use CA certificate from the secret for HTTP client")

			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM(pemBytes)

			tlsClientConfig.RootCAs = certPool
			tlsClientConfig.InsecureSkipVerify = false
		}
	}

	httpClient := &http.Client{
		Timeout: options.GetHttpTimeout(),
		Transport: &http.Transport{
			TLSClientConfig: tlsClientConfig,
		},
	}

	return httpClient, nil
}
