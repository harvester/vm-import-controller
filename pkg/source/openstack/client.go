package openstack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v2/volumes"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/imagedata"
	"github.com/gophercloud/gophercloud/v2/openstack/image/v2/images"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/v2/openstack/utils"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirt "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
	"github.com/harvester/vm-import-controller/pkg/util"
)

const (
	NotUniqueName         = "notUniqueName"
	NotServerFound        = "noServerFound"
	pollingTimeout        = 2 * 60 * 60 * time.Second
	annotationDescription = "field.cattle.io/description"
	computeMicroversion   = "2.19"
)

type Client struct {
	ctx           context.Context
	pClient       *gophercloud.ProviderClient
	opts          gophercloud.EndpointOpts
	storageClient *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient
	imageClient   *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
	options       migration.OpenstackSourceOptions
}

type ExtendedVolume struct {
	VolumeImageMetadata map[string]string `json:"volume_image_metadata,omitempty"`
}

// ExtendedServer The original `Server` structure does not contain the `Description` field.
// References:
// - https://github.com/gophercloud/gophercloud/pull/1505
// - https://docs.openstack.org/api-ref/compute/?expanded=list-all-metadata-detail%2Ccreate-server-detail#show-server-details
type ExtendedServer struct {
	servers.Server
	ServerDescription
}

type ServerDescription struct {
	// This requires microversion 2.19 or later.
	Description string `json:"description"`
}

// NewClient will generate a GopherCloud client
func NewClient(ctx context.Context, endpoint string, region string, secret *corev1.Secret, options migration.OpenstackSourceOptions) (*Client, error) {
	username, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("no username provided in secret %s", secret.Name)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("no password provided in secret %s", secret.Name)
	}

	projectName, ok := secret.Data["project_name"]
	if !ok {
		return nil, fmt.Errorf("no project_name provided in secret %s", secret.Name)
	}

	domainName, ok := secret.Data["domain_name"]
	if !ok {
		return nil, fmt.Errorf("no domain_name provided in secret %s", secret.Name)
	}

	tlsClientConfig := &tls.Config{}

	customCA, ok := secret.Data["ca_cert"]
	if ok {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(customCA)
		tlsClientConfig.RootCAs = caCertPool
	} else {
		tlsClientConfig.InsecureSkipVerify = true
	}

	tr := &http.Transport{TLSClientConfig: tlsClientConfig}

	authOpts := gophercloud.AuthOptions{
		IdentityEndpoint: endpoint,
		Username:         string(username),
		Password:         string(password),
		TenantName:       string(projectName),
		DomainName:       string(domainName),
	}

	endPointOpts := gophercloud.EndpointOpts{
		Region: region,
	}

	client, err := openstack.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error generating new client: %v", err)
	}
	client.HTTPClient.Transport = tr
	err = openstack.Authenticate(ctx, client, authOpts)
	if err != nil {
		return nil, fmt.Errorf("error authenticated client: %v", err)
	}

	storageClient, err := openstack.NewBlockStorageV3(client, endPointOpts)
	if err != nil {
		return nil, fmt.Errorf("error generating storage client: %v", err)
	}

	computeClient, err := openstack.NewComputeV2(client, endPointOpts)
	if err != nil {
		return nil, fmt.Errorf("error generating compute client: %v", err)
	}

	// Try to set the `compute` microversion to 2.19 to get the server description.
	// https://docs.openstack.org/nova/latest/reference/api-microversion-history.html
	supportedMicroversions, err := utils.GetSupportedMicroversions(ctx, computeClient)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch supported microversions from compute client: %v", err)
	}
	supported, err := supportedMicroversions.IsSupported(computeMicroversion)
	if err == nil && supported {
		logrus.WithFields(logrus.Fields{
			"type":            computeClient.Type,
			"minMicroversion": fmt.Sprintf("%d.%d", supportedMicroversions.MinMajor, supportedMicroversions.MinMinor),
			"maxMicroversion": fmt.Sprintf("%d.%d", supportedMicroversions.MaxMajor, supportedMicroversions.MaxMinor),
			"microversion":    computeMicroversion,
		}).Debug("Setting custom microversion")
		computeClient.Microversion = computeMicroversion
	}

	imageClient, err := openstack.NewImageV2(client, endPointOpts)
	if err != nil {
		return nil, fmt.Errorf("error generating image client: %v", err)
	}

	networkClient, err := openstack.NewNetworkV2(client, endPointOpts)
	if err != nil {
		return nil, fmt.Errorf("error generating network client: %v", err)
	}

	return &Client{
		ctx:           ctx,
		pClient:       client,
		opts:          endPointOpts,
		storageClient: storageClient,
		computeClient: computeClient,
		imageClient:   imageClient,
		networkClient: networkClient,
		options:       options,
	}, nil
}

func (c *Client) Verify() error {
	pg := servers.List(c.computeClient, servers.ListOpts{})
	allPg, err := pg.AllPages(c.ctx)
	if err != nil {
		return fmt.Errorf("error generating all pages: %v", err)
	}

	ok, err := allPg.IsEmpty()
	if err != nil {
		return fmt.Errorf("error checking if pages were empty: %v", err)
	}

	if ok {
		return nil
	}

	allServers, err := servers.ExtractServers(allPg)
	if err != nil {
		return fmt.Errorf("error extracting servers: %v", err)
	}

	logrus.Infof("found %d servers", len(allServers))
	return nil
}

func (c *Client) PreFlightChecks(vm *migration.VirtualMachineImport) (err error) {
	if ptr.Deref(vm.Spec.ForcePowerOff, false) {
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Warn("A forced power off is not supported by OpenStack; ignoring the 'ForcePowerOff' setting")
	}

	// Check the source network mappings.
	for _, nm := range vm.Spec.Mapping {
		logrus.WithFields(logrus.Fields{
			"name":          vm.Name,
			"namespace":     vm.Namespace,
			"sourceNetwork": nm.SourceNetwork,
		}).Info("Checking the source network as part of the preflight checks")

		pgr := networks.List(c.networkClient, networks.ListOpts{Name: nm.SourceNetwork})
		allPgs, err := pgr.AllPages(c.ctx)
		if err != nil {
			return fmt.Errorf("error while generating all pages during querying source network '%s': %v", nm.SourceNetwork, err)
		}
		ok, err := allPgs.IsEmpty()
		if err != nil {
			return fmt.Errorf("error while checking if pages were empty during querying source network '%s': %v", nm.SourceNetwork, err)
		}
		if ok {
			return fmt.Errorf("source network '%s' not found", nm.SourceNetwork)
		}
	}

	return nil
}

func (c *Client) ExportVirtualMachine(vm *migration.VirtualMachineImport) error {
	vmObj, err := c.findVM(vm.Spec.VirtualMachineName)
	if err != nil {
		return err
	}

	logrus.WithFields(util.FieldsToJSON(logrus.Fields{
		"name":      vm.Name,
		"namespace": vm.Namespace,
		"spec":      vmObj.AttachedVolumes,
	}, []string{"spec"})).Info("Origin spec of the volumes to be imported")

	// Helper function to do the export.
	// This is necessary so that the defer functions are executed at the right
	// time.
	exportFn := func(index int, av servers.AttachedVolume) error {
		var snapshot *snapshots.Snapshot
		var volume *volumes.Volume
		var volumeImage volumes.VolumeImage

		imageName := fmt.Sprintf("import-controller-%s-%d", vm.Spec.VirtualMachineName, index)

		// Make sure the snapshot, volume and volume image are cleaned up in any case.
		defer func() {
			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.virtualMachineName": vm.Spec.VirtualMachineName,
				"snapshot.id":             ptr.Deref(snapshot, snapshots.Snapshot{}).ID,
				"volume.id":               ptr.Deref(volume, volumes.Volume{}).ID,
				"volumeImage.imageID":     volumeImage.ImageID,
			}).Info("Cleaning up resources on OpenStack source")

			if len(volumeImage.ImageID) > 0 {
				if err := images.Delete(c.ctx, c.imageClient, volumeImage.ImageID).ExtractErr(); err != nil {
					logrus.WithFields(logrus.Fields{
						"name":                    vm.Name,
						"namespace":               vm.Namespace,
						"spec.virtualMachineName": vm.Spec.VirtualMachineName,
						"image.id":                volumeImage.ImageID,
					}).Errorf("Failed to delete image: %v", err)
				}
			}

			if volume != nil {
				if err := volumes.Delete(c.ctx, c.storageClient, volume.ID, volumes.DeleteOpts{}).ExtractErr(); err != nil {
					logrus.WithFields(logrus.Fields{
						"name":                    vm.Name,
						"namespace":               vm.Namespace,
						"spec.virtualMachineName": vm.Spec.VirtualMachineName,
						"volume.id":               volume.ID,
					}).Errorf("Failed to delete volume: %v", err)
				}
			}

			if snapshot != nil {
				if err := snapshots.Delete(c.ctx, c.storageClient, snapshot.ID).ExtractErr(); err != nil {
					logrus.WithFields(logrus.Fields{
						"name":                    vm.Name,
						"namespace":               vm.Namespace,
						"spec.virtualMachineName": vm.Spec.VirtualMachineName,
						"snapshot.id":             snapshot.ID,
						"snapshot.name":           snapshot.Name,
						"snapshot.volumeID":       snapshot.VolumeID,
					}).Errorf("Failed to delete snapshot: %v", err)
				}
			}
		}()

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"opts.name":               imageName,
			"opts.volumeID":           av.ID,
		}).Info("Creating a new snapshot")

		// create snapshot for volume
		snapshot, err = snapshots.Create(c.ctx, c.storageClient, snapshots.CreateOpts{
			Name:     imageName,
			VolumeID: av.ID,
			Force:    true,
		}).Extract()
		// snapshot creation is async, so call returns a 202 error when successful.
		// this is ignored
		if err != nil {
			return err
		}

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"snapshot.id":             snapshot.ID,
			"snapshot.name":           snapshot.Name,
			"snapshot.volumeID":       snapshot.VolumeID,
			"snapshot.size":           snapshot.Size,
		}).Info("Waiting for snapshot to be available")

		ctxWithTimeout1, cancel1 := context.WithTimeout(c.ctx, pollingTimeout)
		defer cancel1()

		if err := snapshots.WaitForStatus(ctxWithTimeout1, c.storageClient, snapshot.ID, "available"); err != nil {
			return fmt.Errorf("timeout waiting for snapshot %s to be available: %w", snapshot.ID, err)
		}

		volume, err = volumes.Create(c.ctx, c.storageClient, volumes.CreateOpts{
			SnapshotID: snapshot.ID,
			Size:       snapshot.Size,
		}, nil).Extract()
		if err != nil {
			return err
		}

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"volume.id":               volume.ID,
			"volume.createdAt":        volume.CreatedAt,
			"volume.snapshotID":       volume.SnapshotID,
			"volume.size":             volume.Size,
			"volume.status":           volume.Status,
			"retryCount":              c.options.UploadImageRetryCount,
			"retryDelay":              c.options.UploadImageRetryDelay,
		}).Info("Waiting for volume to be available")

		ctxWithTimeout2, cancel2 := context.WithTimeout(c.ctx, pollingTimeout)
		defer cancel2()

		if err := volumes.WaitForStatus(ctxWithTimeout2, c.storageClient, volume.ID, "available"); err != nil {
			return fmt.Errorf("timeout waiting for volume %s to be available: %w", volume.ID, err)
		}

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"volume.id":               volume.ID,
			"opts.imagename":          imageName,
		}).Info("Creating a new image from a volume")

		volumeImage, err = volumes.UploadImage(c.ctx, c.storageClient, volume.ID, volumes.UploadImageOpts{
			ImageName:  imageName,
			DiskFormat: "raw",
		}).Extract()
		if err != nil {
			return fmt.Errorf("error while uploading image: %w", err)
		}

		// wait for image to be ready
		isImageActive := false
		for i := 0; i < c.options.UploadImageRetryCount; i++ {
			imgObj, err := images.Get(c.ctx, c.imageClient, volumeImage.ImageID).Extract()
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"name":                    vm.Name,
					"namespace":               vm.Namespace,
					"spec.virtualMachineName": vm.Spec.VirtualMachineName,
					"image.id":                volumeImage.ImageID,
				}).Errorf("Failed to get image: %v", err)
			} else {
				if imgObj.Status == images.ImageStatusActive {
					isImageActive = true
					break
				}
			}

			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.virtualMachineName": vm.Spec.VirtualMachineName,
				"image.id":                imgObj.ID,
				"image.status":            imgObj.Status,
				"retryCount":              c.options.UploadImageRetryCount,
				"retryDelay":              c.options.UploadImageRetryDelay,
				"retryIndex":              i,
			}).Infof("Waiting for image status to be '%s'", images.ImageStatusActive)

			time.Sleep(time.Duration(c.options.UploadImageRetryDelay) * time.Second)
		}
		if !isImageActive {
			return fmt.Errorf("timeout waiting for status '%s' of image %s", images.ImageStatusActive, volumeImage.ImageID)
		}

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"imageID":                 volumeImage.ImageID,
		}).Info("Downloading an image")

		contents, err := imagedata.Download(c.ctx, c.imageClient, volumeImage.ImageID).Extract()
		if err != nil {
			return fmt.Errorf("error downloading image %s: %w", volumeImage.ImageID, err)
		}

		rawImageFileName := generateRawImageFileName(vm.Status.ImportedVirtualMachineName, index)

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"volume.imageID":          volumeImage.ImageID,
			"rawImageFileName":        rawImageFileName,
		}).Info("Downloading RAW image")

		err = writeRawImageFile(filepath.Join(server.TempDir(), rawImageFileName), contents)
		if err != nil {
			return fmt.Errorf("error downloading RAW image %s: %w", rawImageFileName, err)
		}

		vm.Status.DiskImportStatus = append(vm.Status.DiskImportStatus, migration.DiskInfo{
			Name:          rawImageFileName,
			DiskSize:      int64(volume.Size),
			DiskLocalPath: server.TempDir(),
			BusType:       vm.GetDefaultDiskBusType(),
		})

		return nil
	}

	for index, av := range vmObj.AttachedVolumes {
		err := exportFn(index, av)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) ShutdownGuest(vm *migration.VirtualMachineImport) error {
	serverUUID, err := c.checkOrGetUUID(vm.Spec.VirtualMachineName)
	if err != nil {
		return err
	}

	ok, err := c.IsPoweredOff(vm)
	if err != nil {
		return err
	}
	if !ok {
		return servers.Stop(c.ctx, c.computeClient, serverUUID).ExtractErr()
	}
	return nil
}

// PowerOff should never be called for OpenStack but must be implemented due
// to the interface specification.
func (c *Client) PowerOff(_ *migration.VirtualMachineImport) error {
	// Explicitly powering off a VM is not supported by the OpenStack API.
	// Instead, the OpenStack's stop command attempts a graceful shutdown
	// via ACPI, falling back to a forced shutdown if the guest OS does not
	// shut down within a configured timeout.
	return fmt.Errorf("powering off is not supported by OpenStack")
}

func (c *Client) IsPowerOffSupported() bool {
	return false
}

func (c *Client) IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error) {
	s, err := c.findVM(vm.Spec.VirtualMachineName)
	if err != nil {
		return false, err
	}

	if s.Status == "SHUTOFF" {
		return true, nil
	}

	return false, nil
}

func (c *Client) GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error) {
	vmObj, err := c.findVM(vm.Spec.VirtualMachineName)
	if err != nil {
		return nil, fmt.Errorf("error finding VM in GenerateVirtualMachine: %v", err)
	}

	// Log the origin VM specification for better troubleshooting.
	// Note, JSON is used to be able to prettify the output for better readability.
	logrus.WithFields(util.FieldsToJSON(logrus.Fields{
		"name":      vm.Name,
		"namespace": vm.Namespace,
		"spec":      vmObj,
	}, []string{"spec"})).Info("Origin spec of the VM to be imported")

	flavorObj, err := flavors.Get(c.ctx, c.computeClient, vmObj.Flavor["id"].(string)).Extract()
	if err != nil {
		return nil, fmt.Errorf("error looking up flavor: %v", err)
	}

	uefi, tpm, secureBoot, err := c.ImageFirmwareSettings(&vmObj.Server)
	if err != nil {
		return nil, fmt.Errorf("error getting firware settings: %v", err)
	}

	networkInfos, err := generateNetworkInfos(vmObj.Addresses, vm.GetDefaultNetworkInterfaceModel())
	if err != nil {
		return nil, err
	}

	newVM := &kubevirt.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.Status.ImportedVirtualMachineName,
			Namespace: vm.Namespace,
		},
	}

	if vmObj.Description != "" {
		if newVM.Annotations == nil {
			newVM.Annotations = make(map[string]string)
		}
		newVM.Annotations[annotationDescription] = vmObj.Description
	}

	vmSpec := kubevirt.VirtualMachineSpec{
		RunStrategy: &[]kubevirt.VirtualMachineRunStrategy{kubevirt.RunStrategyRerunOnFailure}[0],
		Template: &kubevirt.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"harvesterhci.io/vmName": vm.Status.ImportedVirtualMachineName,
				},
			},
			Spec: kubevirt.VirtualMachineInstanceSpec{
				Domain: kubevirt.DomainSpec{
					CPU: &kubevirt.CPU{
						Cores:   uint32(flavorObj.VCPUs), // nolint:gosec
						Sockets: uint32(1),
						Threads: 1,
					},
					Memory: &kubevirt.Memory{
						Guest: &[]resource.Quantity{resource.MustParse(fmt.Sprintf("%dM", flavorObj.RAM))}[0],
					},
					Resources: kubevirt.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dM", flavorObj.RAM)),
							corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", flavorObj.VCPUs)),
						},
					},
					Features: &kubevirt.Features{
						ACPI: kubevirt.FeatureState{
							Enabled: ptr.To(true),
						},
					},
				},
			},
		},
	}

	mappedNetwork := source.MapNetworks(networkInfos, vm.Spec.Mapping)
	networkConfig, interfaceConfig := source.GenerateNetworkInterfaceConfigs(mappedNetwork, vm.GetDefaultNetworkInterfaceModel())

	// Setup BIOS/EFI, SecureBoot and TPM settings.
	if uefi {
		source.VMSpecSetupUEFISettings(&vmSpec, secureBoot, tpm)
	}

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaceConfig
	newVM.Spec = vmSpec

	// disk attachment needs query by core controller for storage classes, so will be added by the migration controller
	return newVM, nil
}

// SetupOpenStackSecretFromEnv is a helper function to ease with testing
func SetupOpenstackSecretFromEnv(name string) (*corev1.Secret, error) {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}

	username, ok := os.LookupEnv("OS_USERNAME")
	if !ok {
		return nil, fmt.Errorf("no env variable OS_USERNAME specified")
	}

	password, ok := os.LookupEnv("OS_PASSWORD")
	if !ok {
		return nil, fmt.Errorf("no env variable OS_PASSWORD specified")
	}

	tenant, ok := os.LookupEnv("OS_PROJECT_NAME")
	if !ok {
		return nil, fmt.Errorf("no env variable OS_PROJECT_NAME specified")
	}

	domain, ok := os.LookupEnv("OS_USER_DOMAIN_NAME")
	if !ok {
		return nil, fmt.Errorf("no env variable OS_DOMAIN_NAME specified")
	}

	// generate common secret
	data := map[string][]byte{
		"username":     []byte(username),
		"password":     []byte(password),
		"project_name": []byte(tenant),
		"domain_name":  []byte(domain),
	}
	s.Data = data
	return s, nil
}

// SetupOpenstackSourceFromEnv is a helper function to ease with testing
func SetupOpenstackSourceFromEnv() (string, string, error) {
	var endpoint, region string
	var ok bool
	endpoint, ok = os.LookupEnv("OS_AUTH_URL")
	if !ok {
		return endpoint, region, fmt.Errorf("no env variable OS_AUTH_URL specified")
	}

	region, ok = os.LookupEnv("OS_REGION_NAME")
	if !ok {
		return endpoint, region, fmt.Errorf("no env variable OS_AUTH_URL specified")
	}

	return endpoint, region, nil
}

// checkOrGetUUID will check if input is a valid uuid. If not, it assume that the given input
// is a servername and will try and find a uuid for this server.
// openstack allows multiple server names to have the same name, in which case an error will be returned
func (c *Client) checkOrGetUUID(input string) (string, error) {
	parsedUUID, err := uuid.Parse(input)
	if err == nil {
		return parsedUUID.String(), nil
	}

	// assume this is a name and find server based on name
	/*computeClient, err := openstack.NewComputeV2(c.pClient, c.opts)
	if err != nil {
		return "", fmt.Errorf("error generating compute client during checkorGetUUID: %v", err)
	}*/

	pg := servers.List(c.computeClient, servers.ListOpts{Name: input})
	allPg, err := pg.AllPages(c.ctx)
	if err != nil {
		return "", fmt.Errorf("error generating all pages in checkorgetuuid :%v", err)
	}

	ok, err := allPg.IsEmpty()
	if err != nil {
		return "", fmt.Errorf("error checking if pages were empty in checkorgetuuid: %v", err)
	}

	if ok {
		return "", fmt.Errorf(NotServerFound)
	}

	allServers, err := servers.ExtractServers(allPg)
	if err != nil {
		return "", fmt.Errorf("error extracting servers in checkorgetuuid:%v", err)
	}

	// api could return multiple servers matching the pattern of name
	// eg server names test and testvm will match name search "test"
	// in which case we need to filter on actual name
	var filteredServers []servers.Server
	for _, v := range allServers {
		if v.Name == input {
			filteredServers = append(filteredServers, v)
		}
	}

	if len(filteredServers) > 1 {
		return "", fmt.Errorf(NotUniqueName)
	}
	return filteredServers[0].ID, nil
}

func (c *Client) findVM(name string) (*ExtendedServer, error) {
	parsedUUID, err := c.checkOrGetUUID(name)
	if err != nil {
		return nil, err
	}
	sr := servers.Get(c.ctx, c.computeClient, parsedUUID)

	var s ExtendedServer
	err = sr.ExtractInto(&s)

	return &s, err
}

func (c *Client) ImageFirmwareSettings(instance *servers.Server) (bool, bool, bool, error) {
	var imageID string
	var uefiType, tpmEnabled, secureBoot bool
	for _, v := range instance.AttachedVolumes {
		resp := volumes.Get(c.ctx, c.storageClient, v.ID)
		var volInfo volumes.Volume
		if err := resp.ExtractIntoStructPtr(&volInfo, "volume"); err != nil {
			return uefiType, tpmEnabled, secureBoot, fmt.Errorf("error extracting volume info for volume %s: %v", v.ID, err)
		}

		if volInfo.Bootable == "true" {
			var volStatus ExtendedVolume
			if err := resp.ExtractIntoStructPtr(&volStatus, "volume"); err != nil {
				return uefiType, tpmEnabled, secureBoot, fmt.Errorf("error extracting volume status for volume %s: %v", v.ID, err)
			}
			imageID = volStatus.VolumeImageMetadata["image_id"]
		}
	}

	imageInfo, err := images.Get(c.ctx, c.imageClient, imageID).Extract()
	if err != nil {
		return uefiType, tpmEnabled, secureBoot, fmt.Errorf("error getting image details for image %s: %v", imageID, err)
	}
	firmwareType, ok := imageInfo.Properties["hw_firmware_type"]
	if ok && firmwareType.(string) == "uefi" {
		uefiType = true
	}
	logrus.Debugf("found image firmware settings %v", imageInfo.Properties)
	if _, ok := imageInfo.Properties["hw_tpm_model"]; ok {
		tpmEnabled = true
	}

	if val, ok := imageInfo.Properties["os_secure_boot"]; ok && (val == "required" || val == "optional") {
		secureBoot = true
	}
	return uefiType, tpmEnabled, secureBoot, nil
}

func generateNetworkInfos(info map[string]interface{}, defaultInterfaceModel string) ([]source.NetworkInfo, error) {
	networkInfos := make([]source.NetworkInfo, 0)
	uniqueNetworks := make([]source.NetworkInfo, 0)

	for network, values := range info {
		valArr, ok := values.([]interface{})
		if !ok {
			return nil, fmt.Errorf("error asserting interface []interface")
		}
		for _, v := range valArr {
			valMap, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("error asserting network array element into map[string]string")
			}
			networkInfos = append(networkInfos, source.NetworkInfo{
				NetworkName: network,
				MAC:         valMap["OS-EXT-IPS-MAC:mac_addr"].(string),
				// Note, the interface model is not provided via the OpenStack
				// Nova API, therefore we need to set it ourselves.
				Model: defaultInterfaceModel,
			})
		}
	}

	// in case of interfaces with ipv6 and ipv4 addresses they are reported twice, so we need to dedup them
	// based on a mac address
	networksMap := make(map[string]source.NetworkInfo)
	for _, v := range networkInfos {
		networksMap[v.MAC] = v
	}

	for _, v := range networksMap {
		uniqueNetworks = append(uniqueNetworks, v)
	}

	return uniqueNetworks, nil
}

// SanitizeVirtualMachineImport is used to sanitize the VirtualMachineImport object.
func (c *Client) SanitizeVirtualMachineImport(vm *migration.VirtualMachineImport) error {
	// If the given `spec.virtualMachineName` is a UUID, then we need to
	// get the name from the OpenStack server object.
	parsedUUID, err := uuid.Parse(vm.Spec.VirtualMachineName)
	if err == nil {
		vmObj, err := c.findVM(parsedUUID.String())
		if err != nil {
			return err
		}
		vm.Status.ImportedVirtualMachineName = vmObj.Name
	} else {
		vm.Status.ImportedVirtualMachineName = vm.Spec.VirtualMachineName
	}

	// Note, server objects might have upper case characters in OpenStack,
	// so we need to convert them to lower case to be RFC 1123 compliant.
	vm.Status.ImportedVirtualMachineName = strings.ToLower(vm.Status.ImportedVirtualMachineName)

	return nil
}

// writeRawImageFile Download and write the raw image file to the specified path in chunks of 32KiB.
func writeRawImageFile(name string, src io.ReadCloser) error {
	dst, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("error creating raw image file: %v", err)
	}

	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// generateRawImageFileName Generate the raw image file name based on the VM name and index of the attached volume.
func generateRawImageFileName(vmName string, index int) string {
	return fmt.Sprintf("%s-%d.img", vmName, index)
}
