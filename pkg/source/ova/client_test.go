package ova

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/ovf/importer"
	"github.com/vmware/govmomi/vapi/library"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source"
)

var ovfData = `<?xml version="1.0" encoding="UTF-8"?>
<Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1"
          xmlns:ovf="http://schemas.dmtf.org/ovf/envelope/1"
          xmlns:cim="http://schemas.dmtf.org/wbem/wscim/1/common"
          xmlns:rasd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ResourceAllocationSettingData"
          xmlns:vmw="http://www.vmware.com/schema/ovf"
          xmlns:vssd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_VirtualSystemSettingData"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <References>
    <File ovf:id="file1" ovf:href="gm-ubuntu-test-1.vmdk"/>
    <File ovf:id="file2" ovf:href="gm-ubuntu-test-3.nvram" ovf:size="8684"/>
  </References>
  <DiskSection>
    <Info>List of the virtual disks</Info>
    <Disk ovf:capacityAllocationUnits="byte" ovf:format="http://www.vmware.com/interfaces/specifications/vmdk.html#streamOptimized" ovf:diskId="vmdisk1" ovf:capacity="42949672960" ovf:fileRef="file1"/>
  </DiskSection>
  <VirtualSystem ovf:id="gm-ubuntu-test">
    <Info>A Virtual system</Info>
    <Name>gm-ubuntu-test</Name>
    <OperatingSystemSection ovf:id="94" vmw:osType="ubuntu64Guest">
      <Info>The operating system installed</Info>
      <Description>Ubuntu Linux (64-bit)</Description>
    </OperatingSystemSection>
    <VirtualHardwareSection ovf:transport="iso">
      <Item>
        <rasd:AllocationUnits>hertz * 10^6</rasd:AllocationUnits>
        <rasd:Description>Number of Virtual CPUs</rasd:Description>
        <rasd:ElementName>2 virtual CPU(s)</rasd:ElementName>
        <rasd:InstanceID>1</rasd:InstanceID>
        <rasd:ResourceType>3</rasd:ResourceType>
        <rasd:VirtualQuantity>2</rasd:VirtualQuantity>
      </Item>
      <Item>
        <rasd:AllocationUnits>byte * 2^30</rasd:AllocationUnits>
        <rasd:Description>Memory Size</rasd:Description>
        <rasd:ElementName>32GB of memory</rasd:ElementName>
        <rasd:InstanceID>2</rasd:InstanceID>
        <rasd:ResourceType>4</rasd:ResourceType>
        <rasd:VirtualQuantity>32</rasd:VirtualQuantity>
      </Item>
      <vmw:Config ovf:required="false" vmw:key="bootOptions.efiSecureBootEnabled" vmw:value="false"/>
      <vmw:Config ovf:required="false" vmw:key="firmware" vmw:value="bios"/>
    </VirtualHardwareSection>
  </VirtualSystem>
</Envelope>`

var ovfData2 = `<?xml version="1.0" encoding="UTF-8"?>
<Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1"
          xmlns:ovf="http://schemas.dmtf.org/ovf/envelope/1"
          xmlns:cim="http://schemas.dmtf.org/wbem/wscim/1/common"
          xmlns:rasd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ResourceAllocationSettingData"
          xmlns:vmw="http://www.vmware.com/schema/ovf"
          xmlns:vssd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_VirtualSystemSettingData"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <References>
    <File ovf:id="file1" ovf:href="gm-ubuntu-test-1.vmdk"/>
    <File ovf:id="file2" ovf:href="gm-ubuntu-test-3.nvram" ovf:size="8684"/>
  </References>
  <DiskSection>
    <Info>List of the virtual disks</Info>
    <Disk ovf:capacityAllocationUnits="byte" ovf:format="http://www.vmware.com/interfaces/specifications/vmdk.html#streamOptimized" ovf:diskId="vmdisk1" ovf:capacity="42949672960" ovf:fileRef="file1"/>
  </DiskSection>
  <NetworkSection>
    <Info>The list of logical networks</Info>
    <Network ovf:name="DSwitch-vCenter-HA-VM-Network">
      <Description>The DSwitch-vCenter-HA-VM-Network network</Description>
    </Network>
  </NetworkSection>
  <VirtualSystem ovf:id="gm-ubuntu-test">
    <Info>A Virtual system</Info>
    <Name>gm-ubuntu-test</Name>
    <OperatingSystemSection ovf:id="94" vmw:osType="ubuntu64Guest">
      <Info>The operating system installed</Info>
      <Description>Ubuntu Linux (64-bit)</Description>
    </OperatingSystemSection>
    <VirtualHardwareSection ovf:transport="iso">
      <Item ovf:required="false">
        <rasd:AutomaticAllocation>false</rasd:AutomaticAllocation>
        <rasd:ElementName>Virtual TPM</rasd:ElementName>
        <rasd:InstanceID>14</rasd:InstanceID>
        <rasd:ResourceSubType>vmware.vtpm</rasd:ResourceSubType>
        <rasd:ResourceType>1</rasd:ResourceType>
      </Item>
      <Item>
        <rasd:AllocationUnits>hertz * 10^6</rasd:AllocationUnits>
        <rasd:Description>Number of Virtual CPUs</rasd:Description>
        <rasd:ElementName>4 virtual CPU(s)</rasd:ElementName>
        <rasd:InstanceID>1</rasd:InstanceID>
        <rasd:ResourceType>3</rasd:ResourceType>
        <rasd:VirtualQuantity>4</rasd:VirtualQuantity>
        <vmw:CoresPerSocket ovf:required="false">1</vmw:CoresPerSocket>
      </Item>
      <Item>
        <rasd:AllocationUnits>byte * 2^20</rasd:AllocationUnits>
        <rasd:Description>Memory Size</rasd:Description>
        <rasd:ElementName>8192MB of memory</rasd:ElementName>
        <rasd:InstanceID>2</rasd:InstanceID>
        <rasd:ResourceType>4</rasd:ResourceType>
        <rasd:VirtualQuantity>8192</rasd:VirtualQuantity>
      </Item>
	  <Item>
        <rasd:AddressOnParent>0</rasd:AddressOnParent>
        <rasd:AutomaticAllocation>true</rasd:AutomaticAllocation>
        <rasd:Connection>DSwitch-vCenter-HA-VM-Network</rasd:Connection>
        <rasd:ElementName>Network adapter 1</rasd:ElementName>
        <rasd:InstanceID>8</rasd:InstanceID>
        <rasd:ResourceSubType>rtl8139</rasd:ResourceSubType>
        <rasd:ResourceType>10</rasd:ResourceType>
        <vmw:Config ovf:required="false" vmw:key="uptCompatibilityEnabled" vmw:value="true"/>
        <vmw:Config ovf:required="false" vmw:key="uptv2Enabled" vmw:value="false"/>
        <vmw:Config ovf:required="false" vmw:key="slotInfo.pciSlotNumber" vmw:value="192"/>
        <vmw:Config ovf:required="false" vmw:key="wakeOnLanEnabled" vmw:value="true"/>
        <vmw:Config ovf:required="false" vmw:key="connectable.allowGuestControl" vmw:value="false"/>
      </Item>
      <Item>
        <rasd:Address>0</rasd:Address>
        <rasd:Description>SCSI Controller</rasd:Description>
        <rasd:ElementName>SCSI Controller 1</rasd:ElementName>
        <rasd:InstanceID>3</rasd:InstanceID>
        <rasd:ResourceSubType>VirtualSCSI</rasd:ResourceSubType>
        <rasd:ResourceType>6</rasd:ResourceType>
        <vmw:Config ovf:required="false" vmw:key="slotInfo.pciSlotNumber" vmw:value="160"/>
      </Item>
      <Item>
        <rasd:AddressOnParent>0</rasd:AddressOnParent>
        <rasd:ElementName>Hard Disk 1</rasd:ElementName>
        <rasd:HostResource>ovf:/disk/vmdisk1</rasd:HostResource>
        <rasd:InstanceID>5</rasd:InstanceID>
        <rasd:Parent>3</rasd:Parent>
        <rasd:ResourceType>17</rasd:ResourceType>
        <vmw:Config ovf:required="false" vmw:key="guestReadOnly" vmw:value="false"/>
      </Item>
      <vmw:Config ovf:required="false" vmw:key="bootOptions.efiSecureBootEnabled" vmw:value="true"/>
      <vmw:Config ovf:required="false" vmw:key="firmware" vmw:value="efi"/>
    </VirtualHardwareSection>
  </VirtualSystem>
</Envelope>`

var ovfData3 = `<?xml version="1.0"?>
<Envelope ovf:version="2.0"
          xml:lang="en-US"
          xmlns="http://schemas.dmtf.org/ovf/envelope/2"
          xmlns:ovf="http://schemas.dmtf.org/ovf/envelope/2"
          xmlns:rasd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ResourceAllocationSettingData"
          xmlns:vssd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_VirtualSystemSettingData"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xmlns:vbox="http://www.virtualbox.org/ovf/machine"
          xmlns:epasd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_EthernetPortAllocationSettingData.xsd"
          xmlns:sasd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_StorageAllocationSettingData.xsd">
  <References>
    <File ovf:href="ubuntu.2.0-disk1.vmdk" ovf:id="file1"/>
  </References>
  <DiskSection>
    <Info>List of the virtual disks used in the package</Info>
    <Disk ovf:capacity="8589934592" ovf:diskId="vmdisk1" ovf:fileRef="file1" ovf:format="http://www.vmware.com/interfaces/specifications/vmdk.html#streamOptimized" vbox:uuid="ac4b44a7-3ace-401d-bc94-da1b5c3974f3"/>
  </DiskSection>
  <NetworkSection>
    <Info>Logical networks used in the package</Info>
    <Network ovf:name="NAT">
      <Description>Logical network used by this appliance.</Description>
    </Network>
  </NetworkSection>
  <VirtualSystem ovf:id="ubuntu">
    <Info>A virtual machine</Info>
    <OperatingSystemSection ovf:id="94">
      <Info>The kind of installed guest operating system</Info>
      <Description>Ubuntu_64</Description>
      <vbox:OSType ovf:required="false">Ubuntu_64</vbox:OSType>
    </OperatingSystemSection>
    <VirtualHardwareSection>
      <Info>Virtual hardware requirements for a virtual machine</Info>
      <System>
        <vssd:ElementName>Virtual Hardware Family</vssd:ElementName>
        <vssd:InstanceID>0</vssd:InstanceID>
        <vssd:VirtualSystemIdentifier>ubuntu</vssd:VirtualSystemIdentifier>
        <vssd:VirtualSystemType>virtualbox-2.2</vssd:VirtualSystemType>
      </System>
      <Item>
        <rasd:Caption>1 virtual CPU</rasd:Caption>
        <rasd:Description>Number of virtual CPUs</rasd:Description>
        <rasd:InstanceID>1</rasd:InstanceID>
        <rasd:ResourceType>3</rasd:ResourceType>
        <rasd:VirtualQuantity>1</rasd:VirtualQuantity>
      </Item>
      <Item>
        <rasd:AllocationUnits>MegaBytes</rasd:AllocationUnits>
        <rasd:Caption>512 MB of memory</rasd:Caption>
        <rasd:Description>Memory Size</rasd:Description>
        <rasd:InstanceID>2</rasd:InstanceID>
        <rasd:ResourceType>4</rasd:ResourceType>
        <rasd:VirtualQuantity>512</rasd:VirtualQuantity>
      </Item>
      <Item>
        <rasd:Address>0</rasd:Address>
        <rasd:Caption>ideController0</rasd:Caption>
        <rasd:Description>IDE Controller</rasd:Description>
        <rasd:InstanceID>3</rasd:InstanceID>
        <rasd:ResourceSubType>PIIX4</rasd:ResourceSubType>
        <rasd:ResourceType>5</rasd:ResourceType>
      </Item>
      <Item>
        <rasd:Address>1</rasd:Address>
        <rasd:Caption>ideController1</rasd:Caption>
        <rasd:Description>IDE Controller</rasd:Description>
        <rasd:InstanceID>4</rasd:InstanceID>
        <rasd:ResourceSubType>PIIX4</rasd:ResourceSubType>
        <rasd:ResourceType>5</rasd:ResourceType>
      </Item>
      <Item>
        <rasd:Address>0</rasd:Address>
        <rasd:Caption>sataController0</rasd:Caption>
        <rasd:Description>SATA Controller</rasd:Description>
        <rasd:InstanceID>5</rasd:InstanceID>
        <rasd:ResourceSubType>AHCI</rasd:ResourceSubType>
        <rasd:ResourceType>20</rasd:ResourceType>
      </Item>
      <Item>
        <rasd:Address>0</rasd:Address>
        <rasd:Caption>usb</rasd:Caption>
        <rasd:Description>USB Controller</rasd:Description>
        <rasd:InstanceID>6</rasd:InstanceID>
        <rasd:ResourceType>23</rasd:ResourceType>
      </Item>
      <Item>
        <rasd:AddressOnParent>3</rasd:AddressOnParent>
        <rasd:AutomaticAllocation>false</rasd:AutomaticAllocation>
        <rasd:Caption>sound</rasd:Caption>
        <rasd:Description>Sound Card</rasd:Description>
        <rasd:InstanceID>7</rasd:InstanceID>
        <rasd:ResourceSubType>ensoniq1371</rasd:ResourceSubType>
        <rasd:ResourceType>35</rasd:ResourceType>
      </Item>
      <StorageItem>
        <sasd:AddressOnParent>0</sasd:AddressOnParent>
        <sasd:Caption>disk1</sasd:Caption>
        <sasd:Description>Disk Image</sasd:Description>
        <sasd:HostResource>/disk/vmdisk1</sasd:HostResource>
        <sasd:InstanceID>8</sasd:InstanceID>
        <sasd:Parent>5</sasd:Parent>
        <sasd:ResourceType>17</sasd:ResourceType>
      </StorageItem>
      <StorageItem>
        <sasd:AddressOnParent>0</sasd:AddressOnParent>
        <sasd:AutomaticAllocation>true</sasd:AutomaticAllocation>
        <sasd:Caption>cdrom1</sasd:Caption>
        <sasd:Description>CD-ROM Drive</sasd:Description>
        <sasd:InstanceID>9</sasd:InstanceID>
        <sasd:Parent>4</sasd:Parent>
        <sasd:ResourceType>15</sasd:ResourceType>
      </StorageItem>
      <EthernetPortItem>
        <epasd:AutomaticAllocation>true</epasd:AutomaticAllocation>
        <epasd:Caption>Ethernet adapter on 'NAT'</epasd:Caption>
        <epasd:Connection>NAT</epasd:Connection>
        <epasd:InstanceID>10</epasd:InstanceID>
        <epasd:ResourceSubType>E1000</epasd:ResourceSubType>
        <epasd:ResourceType>10</epasd:ResourceType>
      </EthernetPortItem>
    </VirtualHardwareSection>
  </VirtualSystem>
</Envelope>`

func Test_NewClient(t *testing.T) {
	assert := require.New(t)

	c, err := NewClient(context.TODO(), "https://harvesterhci.io/", nil, migration.OvaSourceOptions{})
	assert.NoError(err, "expected no error during creation of client")
	assert.NotNil(c.httpClient, "expected client to not be nil")
	assert.Equal(server.TempDir(), c.workingDir, "expected working dir to match")

	httpTransport := c.httpClient.Transport.(*http.Transport)
	assert.True(httpTransport.TLSClientConfig.InsecureSkipVerify, "expected InsecureSkipVerify to be true")
	assert.Nil(httpTransport.TLSClientConfig.RootCAs, "expected RootCAs not to be set")
	assert.Equal(time.Duration(migration.DefaultHttpTimeoutSeconds)*time.Second, c.httpClient.Timeout, "expected http client timeout to be 5 minutes")

	err = c.Verify()
	assert.NoError(err, "expected no error during verification of client")
}

func Test_NewClient_CACert(t *testing.T) {
	assert := require.New(t)
	secret := &corev1.Secret{
		Data: map[string][]byte{
			"ca.crt": []byte(`-----BEGIN CERTIFICATE-----xyz-----END CERTIFICATE-----`),
		},
	}
	httpTimeoutSeconds := 10

	c, err := NewClient(context.TODO(), "https://foobar.com/", secret, migration.OvaSourceOptions{
		HttpTimeoutSeconds: ptr.To(httpTimeoutSeconds),
	})
	assert.NoError(err, "expected no error during creation of client")
	assert.NotNil(c.httpClient, "expected client to not be nil")
	assert.Equal(time.Duration(httpTimeoutSeconds)*time.Second, c.httpClient.Timeout, "expected http client timeout to be 10 seconds")

	httpTransport := c.httpClient.Transport.(*http.Transport)
	assert.False(httpTransport.TLSClientConfig.InsecureSkipVerify, "expected InsecureSkipVerify to be false")
	assert.NotNil(httpTransport.TLSClientConfig.RootCAs, "expected RootCAs to be set")
}

func Test_PowerOff(t *testing.T) {
	assert := require.New(t)

	c, err := NewClient(context.TODO(), "https://harvesterhci.io/", nil, migration.OvaSourceOptions{})
	assert.NoError(err, "expected no error during creation of client")
	supported := c.IsPowerOffSupported()
	assert.False(supported, "expected powering off is not supported")
	err = c.PowerOff(&migration.VirtualMachineImport{})
	assert.NoError(err, "expected no error during VM power off")
}

func Test_ShutdownGuest(t *testing.T) {
	assert := require.New(t)

	c, err := NewClient(context.TODO(), "https://harvesterhci.io/", nil, migration.OvaSourceOptions{})
	assert.NoError(err, "expected no error during creation of client")
	err = c.ShutdownGuest(&migration.VirtualMachineImport{})
	assert.NoError(err, "expected no error during VM shutdown via guest OS")
}

func Test_readManifest(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		manifest string
		expected map[string]*library.Checksum
	}{
		{
			desc: "Test 1",
			manifest: `
SHA1(wazuh-4.13.1-disk-1.vmdk) = 7c1af154f974a4dd448dc527f1f4216491973039
SHA1(wazuh-4.13.1.ovf) = d861e788a9f65975b6c51686b721792985db88c7
`,
			expected: map[string]*library.Checksum{
				"wazuh-4.13.1-disk-1.vmdk": ptr.To(library.Checksum{
					Algorithm: "SHA1",
					Checksum:  "7c1af154f974a4dd448dc527f1f4216491973039",
				}),
				"wazuh-4.13.1.ovf": ptr.To(library.Checksum{
					Algorithm: "SHA1",
					Checksum:  "d861e788a9f65975b6c51686b721792985db88c7",
				}),
			},
		},
		{
			desc: "Test 2",
			manifest: `
SHA256(ubuntu.2.0.ovf)= 4aacc96f73bc1e0912414b80a576f62fa8d22386a2c34c489e88ee42ec71de9b
SHA256(ubuntu.2.0-disk1.vmdk)= 4a218c15a1e8aed26cb0a2a533562e85a9f28956a6666181d0c9bb7ba58b5b06
`,
			expected: map[string]*library.Checksum{
				"ubuntu.2.0.ovf": ptr.To(library.Checksum{
					Algorithm: "SHA256",
					Checksum:  "4aacc96f73bc1e0912414b80a576f62fa8d22386a2c34c489e88ee42ec71de9b",
				}),
				"ubuntu.2.0-disk1.vmdk": ptr.To(library.Checksum{
					Algorithm: "SHA256",
					Checksum:  "4a218c15a1e8aed26cb0a2a533562e85a9f28956a6666181d0c9bb7ba58b5b06",
				}),
			},
		},
	}

	for _, tc := range testCases {
		reader := strings.NewReader(tc.manifest)
		result, err := readManifest(reader)
		assert.NoError(err, "expected no error during reading manifest")
		assert.Equal(tc.expected, result, "expected manifest to match")
	}
}

func Test_readEnvelope(t *testing.T) {
	assert := require.New(t)

	_, currentFile, _, _ := runtime.Caller(0)
	pwd := filepath.Dir(currentFile)

	e, err := readEnvelope(filepath.Join(pwd, "test.ova"))
	assert.NoError(err, "expected no error during reading envelope")
	assert.NotNil(e, "expected envelope to be returned")
	assert.Equal("ubuntu.2.0-disk1.vmdk", e.References[0].Href, "expected href to match")
	assert.Len(e.Disk.Disks, 1, "expected one disk")
	assert.Len(e.Network.Networks, 1, "expected one network")
	assert.Equal("Ubuntu_64", *e.VirtualSystem.OperatingSystem.Description, "expected OS description to match")
}

func Test_parseCapacity(t *testing.T) {
	assert := require.New(t)

	capacity := parseCapacity(ovf.VirtualDiskDesc{Capacity: "42949672960"})
	assert.Equal(int64(42949672960), capacity)

	capacity = parseCapacity(ovf.VirtualDiskDesc{Capacity: "5", CapacityAllocationUnits: ptr.To("MegaBytes")})
	assert.Equal(int64(5242880), capacity)

	capacity = parseCapacity(ovf.VirtualDiskDesc{Capacity: "42949672960", CapacityAllocationUnits: ptr.To("byte")})
	assert.Equal(int64(42949672960), capacity)

	capacity = parseCapacity(ovf.VirtualDiskDesc{Capacity: "10", CapacityAllocationUnits: ptr.To("byte * 2^30")})
	assert.Equal(int64(10737418240), capacity)

	capacity = parseCapacity(ovf.VirtualDiskDesc{Capacity: "10", CapacityAllocationUnits: ptr.To("GB")})
	assert.Equal(int64(10737418240), capacity)

	capacity = parseCapacity(ovf.VirtualDiskDesc{Capacity: "5", CapacityAllocationUnits: ptr.To("byte * 2^20")})
	assert.Equal(int64(5242880), capacity)
}

func Test_parseVirtualQuantity(t *testing.T) {
	assert := require.New(t)

	quantity := parseVirtualQuantity(ovf.ResourceAllocationSettingData{
		CIMResourceAllocationSettingData: ovf.CIMResourceAllocationSettingData{
			VirtualQuantity: ptr.To(uint(42949672960)),
		},
	})
	assert.Equal(int64(42949672960), quantity)

	quantity = parseVirtualQuantity(ovf.ResourceAllocationSettingData{
		CIMResourceAllocationSettingData: ovf.CIMResourceAllocationSettingData{
			VirtualQuantity: ptr.To(uint(42949672960)),
			AllocationUnits: ptr.To("byte"),
		},
	})
	assert.Equal(int64(42949672960), quantity)

	quantity = parseVirtualQuantity(ovf.ResourceAllocationSettingData{
		CIMResourceAllocationSettingData: ovf.CIMResourceAllocationSettingData{
			VirtualQuantity: ptr.To(uint(10)),
			AllocationUnits: ptr.To("byte * 2^30"),
		},
	})
	assert.Equal(int64(10737418240), quantity)

	quantity = parseVirtualQuantity(ovf.ResourceAllocationSettingData{
		CIMResourceAllocationSettingData: ovf.CIMResourceAllocationSettingData{
			VirtualQuantity: ptr.To(uint(10)),
			AllocationUnits: ptr.To("GigaBytes"),
		},
	})
	assert.Equal(int64(10737418240), quantity)

	quantity = parseVirtualQuantity(ovf.ResourceAllocationSettingData{
		CIMResourceAllocationSettingData: ovf.CIMResourceAllocationSettingData{
			VirtualQuantity: ptr.To(uint(5)),
			AllocationUnits: ptr.To("byte * 2^20"),
		},
	})
	assert.Equal(int64(5242880), quantity)

	quantity = parseVirtualQuantity(ovf.ResourceAllocationSettingData{
		CIMResourceAllocationSettingData: ovf.CIMResourceAllocationSettingData{
			VirtualQuantity: ptr.To(uint(5)),
			AllocationUnits: ptr.To("MB"),
		},
	})
	assert.Equal(int64(5242880), quantity)
}

func Test_resolveReference(t *testing.T) {
	assert := require.New(t)
	e, err := importer.ReadEnvelope([]byte(ovfData))
	assert.NoError(err, "expected no error during reading envelope")
	ref := resolveReference(e, "file1")
	assert.NotNil(ref, "expected file reference to be found")
	assert.Equal("gm-ubuntu-test-1.vmdk", ref.Href, "expected href to match")

	ref = resolveReference(e, "file3")
	assert.Nil(ref, "expected reference not found")
}

func Test_findResourceAllocationSettingData(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc       string
		instanceID string
		expected   bool
	}{
		{
			desc:       "Test 1",
			instanceID: "3",
			expected:   true,
		},
		{
			desc:       "Test 2",
			instanceID: "99",
			expected:   false,
		},
	}

	e, err := importer.ReadEnvelope([]byte(ovfData2))
	assert.NoError(err, "expected no error during reading envelope")
	assert.NotNil(e.VirtualSystem.VirtualHardware, "expected virtual hardware section to be present")

	for _, tc := range testCases {
		item := findResourceAllocationSettingData(&e.VirtualSystem.VirtualHardware[0], tc.instanceID)
		if tc.expected {
			assert.NotNil(item, "expected item to be found")
		} else {
			assert.Nil(item, "expected item not to be found")
		}
	}
}

func Test_extractAndConvertVMDKToRAW(t *testing.T) {
	assert := require.New(t)

	_, currentFile, _, _ := runtime.Caller(0)
	pwd := filepath.Dir(currentFile)

	workingDir, err := os.MkdirTemp("", "*")
	assert.NoError(err, "expected no error during creating temp dir")

	c := &Client{
		workingDir: workingDir,
	}
	err = c.extractAndConvertVMDKToRAW(
		filepath.Join(pwd, "test.ova"),
		"ubuntu.2.0-disk1.vmdk",
		"", false)
	assert.NoError(err)

	assert.NoFileExists(filepath.Join(workingDir, "ubuntu.2.0-disk1.vmdk"))
	_ = os.RemoveAll(workingDir)
}

func Test_parseEnvelope_DiskInfo_empty(t *testing.T) {
	assert := require.New(t)
	e := &ovf.Envelope{}
	_, _, _, dis := parseEnvelope(e, "", "foo")
	assert.Len(dis, 0, "expected no disk info")
}

func Test_parseEnvelope_DiskInfo_ovf_v1(t *testing.T) {
	assert := require.New(t)
	e, err := importer.ReadEnvelope([]byte(ovfData2))
	assert.NoError(err, "expected no error during reading envelope")
	_, _, _, dis := parseEnvelope(e, "", "buz")
	assert.Len(dis, 1, "expected one disk info")
	assert.Equal("gm-ubuntu-test-1.vmdk", dis[0].Name, "expected name to match")
	assert.Equal(int64(42949672960), dis[0].DiskSize, "expected size to match")
	assert.Equal(kubevirtv1.DiskBusSCSI, dis[0].BusType, "expected bus type to match")
}

func Test_parseEnvelope_DiskInfo_ovf_v2(t *testing.T) {
	assert := require.New(t)
	e, err := importer.ReadEnvelope([]byte(ovfData3))
	assert.NoError(err, "expected no error during reading envelope")
	_, _, _, dis := parseEnvelope(e, "", "buz")
	assert.Len(dis, 1, "expected one disk info")
	assert.Equal("ubuntu.2.0-disk1.vmdk", dis[0].Name, "expected name to match")
	assert.Equal(int64(8589934592), dis[0].DiskSize, "expected size to match")
	assert.Equal(kubevirtv1.DiskBusSATA, dis[0].BusType, "expected bus type to match")
}

func Test_parseEnvelope_Firmware(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		envelope []byte
		expected *source.Firmware
	}{
		{
			desc:     "Test 1",
			envelope: []byte(ovfData),
			expected: source.NewFirmware(false, false, false),
		},
		{
			desc:     "Test 2",
			envelope: []byte(ovfData2),
			expected: source.NewFirmware(true, true, true),
		},
	}

	for _, tc := range testCases {
		e, err := importer.ReadEnvelope(tc.envelope)
		assert.NoError(err, "expected no error during reading envelope")
		fw, _, _, _ := parseEnvelope(e, "foo", "buz")
		assert.Equal(tc.expected.UEFI, fw.UEFI, "expected UEFI flag to match")
		assert.Equal(tc.expected.TPM, fw.TPM, "expected TPM flag to match")
		assert.Equal(tc.expected.SecureBoot, fw.SecureBoot, "expected SecureBoot flag to match")
	}
}

func Test_parseEnvelope_Hardware(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		envelope []byte
		expected source.Hardware
	}{
		{
			desc:     "Test 1",
			envelope: []byte(ovfData),
			expected: source.Hardware{
				NumCPU:            2,
				NumCoresPerSocket: 0,
				MemoryMB:          32768,
			},
		},
		{
			desc:     "Test 2",
			envelope: []byte(ovfData2),
			expected: source.Hardware{
				NumCPU:            4,
				NumCoresPerSocket: 0,
				MemoryMB:          8192,
			},
		},
	}

	for _, tc := range testCases {
		e, err := importer.ReadEnvelope(tc.envelope)
		assert.NoError(err, "expected no error during reading envelope")
		_, hw, _, _ := parseEnvelope(e, "bar", "buz")
		assert.Equal(tc.expected.NumCPU, hw.NumCPU, "expected CPU count to match")
		assert.Equal(tc.expected.NumCoresPerSocket, hw.NumCoresPerSocket, "expected number of cores per socket to match")
		assert.Equal(tc.expected.MemoryMB, hw.MemoryMB, "expected memory size to match")
	}
}

func Test_parseEnvelope_NetworkInfos_v1(t *testing.T) {
	assert := require.New(t)
	e, err := importer.ReadEnvelope([]byte(ovfData2))
	assert.NoError(err, "expected no error during reading envelope")
	_, _, ni, _ := parseEnvelope(e, "baz", "buz")
	assert.Len(ni, 1)
	assert.Equal("DSwitch-vCenter-HA-VM-Network", ni[0].NetworkName, "expected network name to match")
	assert.Empty(ni[0].MAC, "expected MAC address to be empty")
	assert.Equal(migration.NetworkInterfaceModelRtl8139, ni[0].Model, "expected network model to match")
}

func Test_downloadArchive(t *testing.T) {
	assert := require.New(t)

	_, currentFile, _, _ := runtime.Caller(0)
	pwd := filepath.Dir(currentFile)

	dstFile, err := os.CreateTemp("", "*")
	assert.NoError(err, "expected no error during creating temp file")

	defer func() {
		_ = dstFile.Close()
		_ = os.Remove(dstFile.Name())
	}()

	testCases := []struct {
		desc   string
		secret *corev1.Secret
	}{
		{
			desc:   "Without basic auth",
			secret: nil,
		},
		{
			desc: "With basic auth",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					"username": []byte("foo"),
					"password": []byte("bar"),
				},
			},
		},
	}

	for _, tc := range testCases {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.secret != nil {
				username, password, ok := r.BasicAuth()
				assert.True(ok, "expected basic auth")
				assert.Equal(string(tc.secret.Data["username"]), username, "expected username to match")
				assert.Equal(string(tc.secret.Data["password"]), password, "expected password to match")
			}

			name := filepath.Join(pwd, "test.ova")
			http.ServeFile(w, r, name)
		})

		httpServer := httptest.NewTLSServer(handler)
		defer httpServer.Close()

		if tc.secret != nil {
			// Add CA cert to secret.
			assert.NotNil(httpServer.Certificate())
			pemBytes := pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: httpServer.Certificate().Raw,
			})
			tc.secret.Data["ca.crt"] = pemBytes
		}

		c, _ := NewClient(context.TODO(), httpServer.URL+"/test.ova",
			tc.secret, migration.OvaSourceOptions{})

		err = c.downloadArchive(dstFile.Name())
		assert.NoError(err)

		assert.FileExists(dstFile.Name())
		_ = os.Remove(dstFile.Name())
	}
}
