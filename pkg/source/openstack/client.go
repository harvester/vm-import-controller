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
	"time"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/extensions/volumeactions"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v2/volumes"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/startstop"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/imagedata"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirt "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/qemu"
	"github.com/harvester/vm-import-controller/pkg/server"
)

const (
	NotUniqueName   = "notUniqueName"
	NotServerFound  = "noServerFound"
	defaultInterval = 10 * time.Second
	defaultCount    = 30
)

type Client struct {
	ctx           context.Context
	pClient       *gophercloud.ProviderClient
	opts          gophercloud.EndpointOpts
	storageClient *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient
	imageClient   *gophercloud.ServiceClient
}

// NewClient will generate a GopherCloud client
func NewClient(ctx context.Context, endpoint string, region string, secret *corev1.Secret) (*Client, error) {
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

	config := &tls.Config{}

	customCA, ok := secret.Data["ca_cert"]
	if ok {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(customCA)
		config.RootCAs = caCertPool
	} else {
		config.InsecureSkipVerify = true
	}

	tr := &http.Transport{TLSClientConfig: config}

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
	err = openstack.Authenticate(client, authOpts)
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

	imageClient, err := openstack.NewImageServiceV2(client, endPointOpts)
	if err != nil {
		return nil, fmt.Errorf("error generating image client: %v", err)
	}
	return &Client{
		ctx:           ctx,
		pClient:       client,
		opts:          endPointOpts,
		storageClient: storageClient,
		computeClient: computeClient,
		imageClient:   imageClient,
	}, nil
}

func (c *Client) Verify() error {
	computeClient, err := openstack.NewComputeV2(c.pClient, c.opts)
	if err != nil {
		return fmt.Errorf("error generating compute client during verify phase :%v", err)
	}

	pg := servers.List(computeClient, servers.ListOpts{})
	allPg, err := pg.AllPages()
	if err != nil {
		return fmt.Errorf("error generating all pages :%v", err)
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
		return fmt.Errorf("error extracting servers :%v", err)
	}

	logrus.Infof("found %d servers", len(allServers))
	return nil
}

func (c *Client) ExportVirtualMachine(vm *migration.VirtualMachineImport) error {
	vmObj, err := c.findVM(vm.Spec.VirtualMachineName)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("/tmp", "openstack-image-")
	if err != nil {
		return fmt.Errorf("error creating tmp image directory: %v", err)
	}

	for i, v := range vmObj.AttachedVolumes {
		// create snapshot for volume
		snapInfo, err := snapshots.Create(c.storageClient, snapshots.CreateOpts{
			Name:     fmt.Sprintf("import-controller-%v-%d", vm.Spec.VirtualMachineName, i),
			VolumeID: v.ID,
			Force:    true,
		}).Extract()

		// snapshot creation is async, so call returns a 202 error when successful.
		// this is ignored
		if err != nil {
			return err
		}

		for i := 0; i < defaultCount; i++ {
			snapObj, err := snapshots.Get(c.storageClient, snapInfo.ID).Extract()
			if err != nil {
				return err
			}

			if snapObj.Status == "available" {
				break
			}
			time.Sleep(defaultInterval)
		}

		volObj, err := volumes.Create(c.storageClient, volumes.CreateOpts{
			SnapshotID: snapInfo.ID,
			Size:       snapInfo.Size,
		}).Extract()
		if err != nil {
			return err
		}

		logrus.Info(volObj)

		for i := 0; i < defaultCount; i++ {
			tmpVolObj, err := volumes.Get(c.storageClient, volObj.ID).Extract()
			if err != nil {
				return err
			}
			if tmpVolObj.Status == "available" {
				break
			}
			time.Sleep(defaultInterval)
		}

		logrus.Info("attempting to create new image from volume")

		volImage, err := volumeactions.UploadImage(c.storageClient, volObj.ID, volumeactions.UploadImageOpts{
			ImageName:  fmt.Sprintf("import-controller-%s-%d", vm.Spec.VirtualMachineName, i),
			DiskFormat: "qcow2",
		}).Extract()

		if err != nil {
			return err
		}
		// wait for image to be ready
		for i := 0; i < defaultCount; i++ {
			imgObj, err := images.Get(c.imageClient, volImage.ImageID).Extract()
			if err != nil {
				return fmt.Errorf("error checking status of volume image: %v", err)
			}
			if imgObj.Status == "active" {
				break
			}
			time.Sleep(defaultInterval)
		}

		contents, err := imagedata.Download(c.imageClient, volImage.ImageID).Extract()
		if err != nil {
			return err
		}

		imageContents, err := io.ReadAll(contents)
		if err != nil {
			return err
		}

		qcowFileName := filepath.Join(tmpDir, fmt.Sprintf("%s-%d", vm.Spec.VirtualMachineName, i))
		imgFile, err := os.Create(qcowFileName)
		if err != nil {
			return fmt.Errorf("error creating disk file: %v", err)
		}

		_, err = imgFile.Write(imageContents)
		if err != nil {
			return err
		}
		imgFile.Close()

		// downloaded image is qcow2. Convert to raw file
		rawFileName := filepath.Join(server.TempDir(), fmt.Sprintf("%s-%d.img", vmObj.Name, i))
		err = qemu.ConvertQCOW2toRAW(qcowFileName, rawFileName)
		if err != nil {
			return fmt.Errorf("error converting qcow2 to raw file: %v", err)
		}

		if err := volumes.Delete(c.storageClient, volObj.ID, volumes.DeleteOpts{}).ExtractErr(); err != nil {
			return fmt.Errorf("error deleting volume %s: %v", volObj.ID, err)
		}

		if err := snapshots.Delete(c.storageClient, snapInfo.ID).ExtractErr(); err != nil {
			return fmt.Errorf("error deleting snapshot %s: %v", snapInfo.ID, err)
		}

		if err := images.Delete(c.imageClient, volImage.ImageID).ExtractErr(); err != nil {
			return fmt.Errorf("error deleting image %s: %v", volImage.ImageID, err)
		}

		vm.Status.DiskImportStatus = append(vm.Status.DiskImportStatus, migration.DiskInfo{
			Name:          fmt.Sprintf("%s-%d.img", vmObj.Name, i),
			DiskSize:      int64(volObj.Size),
			DiskLocalPath: server.TempDir(),
		})
	}
	return os.RemoveAll(tmpDir)
}

func (c *Client) PowerOffVirtualMachine(vm *migration.VirtualMachineImport) error {
	computeClient, err := openstack.NewComputeV2(c.pClient, c.opts)
	if err != nil {
		return fmt.Errorf("error generating compute client during poweroffvirtualmachine: %v", err)
	}
	uuid, err := c.checkOrGetUUID(vm.Spec.VirtualMachineName)
	if err != nil {
		return err
	}

	ok, err := c.IsPoweredOff(vm)
	if err != nil {
		return err
	}
	if !ok {
		return startstop.Stop(computeClient, uuid).ExtractErr()
	}
	return nil
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
		return nil, fmt.Errorf("error finding vm in generatevirtualmachine: %v", err)
	}

	flavorObj, err := flavors.Get(c.computeClient, vmObj.Flavor["id"].(string)).Extract()
	if err != nil {
		return nil, fmt.Errorf("error looking up flavor: %v", err)
	}

	var networks []networkInfo
	for network, values := range vmObj.Addresses {
		valArr, ok := values.([]interface{})
		if !ok {
			return nil, fmt.Errorf("error asserting interface []interface")
		}
		for _, v := range valArr {
			valMap, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("error asserting network array element into map[string]string")
			}
			networks = append(networks, networkInfo{
				NetworkName: network,
				MAC:         valMap["OS-EXT-IPS-MAC:mac_addr"].(string),
			})
		}
	}
	newVM := &kubevirt.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vm.Spec.VirtualMachineName,
			Namespace: vm.Namespace,
		},
	}

	vmSpec := kubevirt.VirtualMachineSpec{
		RunStrategy: &[]kubevirt.VirtualMachineRunStrategy{kubevirt.RunStrategyRerunOnFailure}[0],
		Template: &kubevirt.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"harvesterhci.io/vmName": vm.Spec.VirtualMachineName,
				},
			},
			Spec: kubevirt.VirtualMachineInstanceSpec{
				Domain: kubevirt.DomainSpec{
					CPU: &kubevirt.CPU{
						Cores:   uint32(flavorObj.VCPUs),
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
				},
			},
		},
	}

	mappedNetwork := mapNetworkCards(networks, vm.Spec.Mapping)
	networkConfig := make([]kubevirt.Network, 0, len(mappedNetwork))
	for i, v := range mappedNetwork {
		networkConfig = append(networkConfig, kubevirt.Network{
			NetworkSource: kubevirt.NetworkSource{
				Multus: &kubevirt.MultusNetwork{
					NetworkName: v.MappedNetwork,
				},
			},
			Name: fmt.Sprintf("migrated-%d", i),
		})
	}

	interfaces := make([]kubevirt.Interface, 0, len(mappedNetwork))
	for i, v := range mappedNetwork {
		interfaces = append(interfaces, kubevirt.Interface{
			Name:       fmt.Sprintf("migrated-%d", i),
			MacAddress: v.MAC,
			Model:      "virtio",
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Bridge: &kubevirt.InterfaceBridge{},
			},
		})
	}
	// if there is no network, attach to Pod network. Essential for VM to be booted up
	if len(networkConfig) == 0 {
		networkConfig = append(networkConfig, kubevirt.Network{
			Name: "pod-network",
			NetworkSource: kubevirt.NetworkSource{
				Pod: &kubevirt.PodNetwork{},
			},
		})
		interfaces = append(interfaces, kubevirt.Interface{
			Name:  "pod-network",
			Model: "virtio",
			InterfaceBindingMethod: kubevirt.InterfaceBindingMethod{
				Masquerade: &kubevirt.InterfaceMasquerade{},
			},
		})
	}

	vmSpec.Template.Spec.Networks = networkConfig
	vmSpec.Template.Spec.Domain.Devices.Interfaces = interfaces
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
	allPg, err := pg.AllPages()
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

	if len(allServers) > 1 {
		return "", fmt.Errorf(NotUniqueName)
	}

	return allServers[0].ID, nil
}

func (c *Client) findVM(name string) (*servers.Server, error) {
	parsedUUID, err := c.checkOrGetUUID(name)
	if err != nil {
		return nil, err
	}
	return servers.Get(c.computeClient, parsedUUID).Extract()

}

type networkInfo struct {
	NetworkName   string
	MAC           string
	MappedNetwork string
}

func mapNetworkCards(networkCards []networkInfo, mapping []migration.NetworkMapping) []networkInfo {
	var retNetwork []networkInfo
	for _, nc := range networkCards {
		for _, m := range mapping {
			if m.SourceNetwork == nc.NetworkName {
				nc.MappedNetwork = m.DestinationNetwork
				retNetwork = append(retNetwork, nc)
			}
		}
	}

	return retNetwork
}
