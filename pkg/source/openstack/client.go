package openstack

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/sirupsen/logrus"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kubevirt "kubevirt.io/api/core/v1"
)

type Client struct {
	ctx     context.Context
	pClient *gophercloud.ProviderClient
	opts    gophercloud.EndpointOpts
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
	client, err := openstack.AuthenticatedClient(authOpts)
	if err != nil {
		return nil, fmt.Errorf("error authenticated client: %v", err)
	}

	return &Client{
		ctx:     ctx,
		pClient: client,
		opts:    endPointOpts,
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
