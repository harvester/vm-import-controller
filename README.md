# vm-import-controller

vm-import-controller is an addon to help migrate VM workloads from other source clusters to an existing Harvester cluster.

Currently the following source providers will be supported:
* vmware
* openstack

## API
The vm-import-controller introduces two CRD's

### Sources
sources allows users to define valid source clusters.

For example:

```yaml
apiVersion: migration.harvesterhci.io/v1beta1
kind: VmwareSource
metadata:
  name: vcsim
  namespace: default
spec:
  endpoint: "https://vscim/sdk"
  dc: "DCO"
  credentials:
    name: vsphere-credentials
    namespace: default
```

The secret contains the credentials for the vcenter endpoint:

```yaml
apiVersion: v1
kind: Secret
metadata: 
  name: vsphere-credentials
  namespace: default
stringData:
  "username": "user"
  "password": "password"
```

As part of the reconcile process, the controller will login to the vcenter and verify the `dc` specified in the source spec is valid.

Once this check is passed, the source is marked ready, and can be used for vm migrations

```shell
$ kubectl get vmwaresource.migration 
NAME      STATUS
vcsim   clusterReady
```

For openstack based source clusters a sample definition is as follows:

```yaml
apiVersion: migration.harvesterhci.io/v1beta1
kind: OpenstackSource
metadata:
  name: devstack
  namespace: default
spec:
  endpoint: "https://devstack/identity"
  region: "RegionOne"
  credentials:
    name: devstack-credentials
    namespace: default
```

The secret contains the credentials for the vcenter endpoint:

```yaml
apiVersion: v1
kind: Secret
metadata: 
  name: devstack-credentials
  namespace: default
stringData:
  "username": "user"
  "password": "password"
  "project_name": "admin"
  "domain_name": "default"
  "ca_cert": "pem-encoded-ca-cert"
```

Openstack source reconcile process, attempts to list VM's in the project, and marks the source as ready

```shell
$ kubectl get openstacksource.migration
NAME       STATUS
devstack   clusterReady
```

### VirtualMachimeImport
The VirtualMachineImport crd provides a way for users to define the source VM and mapping to the actual source cluster to perform the VM export-import from.

A sample VirtualMachineImport looks as follows:

```yaml
apiVersion: migration.harvesterhci.io/v1beta1
kind: VirtualMachineImport
metadata:
  name: alpine-export-test
  namespace: default
spec: 
  virtualMachineName: "alpine-export-test"
  networkMapping:
  - sourceNetwork: "dvSwitch 1"
    destinationNetwork: "default/vlan1"
  - sourceNetwork: "dvSwitch 2"
    destinationNetwork: "default/vlan2"
  sourceCluster: 
    name: vcsim
    namespace: default
    kind: VmwareSource
    apiVersion: migration.harvesterhci.io/v1beta1
```

This will trigger the controller to export the VM named "alpine-export-test" on the vmware source vcsim to be exported, processed and recreated into the harvester cluster

This can take a while based on the size of the virtual machine, but users should see `VirtualMachineImages` created for each disk in the defined virtual machine.

The list of items in `networkMapping` will define how the source network interfaces are mapped into the Harvester Networks.

If a match is not found, then each unmatched network inteface is attached to the default `managementNetwork`

Once the virtual machine has been imported successfully the object will reflect the status

```shell
$ kubectl get virtualmachineimport.migration
NAME                    STATUS
alpine-export-test      virtualMachineRunning
openstack-cirros-test   virtualMachineRunning

```

Similarly, users can define a VirtualMachineImport for Openstack source as well:

```yaml
apiVersion: migration.harvesterhci.io/v1beta1
kind: VirtualMachineImport
metadata:
  name: openstack-demo
  namespace: default
spec: 
  virtualMachineName: "openstack-demo" #Name or UUID for instance
  networkMapping:
  - sourceNetwork: "shared"
    destinationNetwork: "default/vlan1"
  - sourceNetwork: "public"
    destinationNetwork: "default/vlan2"
  sourceCluster: 
    name: devstack
    namespace: default
    kind: OpenstackSource
    apiVersion: migration.harvesterhci.io/v1beta1
```

*NOTE:* Openstack allows users to have multiple instances with the same name. In such a scenario the users are advised to use the Instance ID. The reconcile logic tries to perform a lookup from name to ID when a name is used.


## Testing
Currently basic integration tests are available under `tests/integration`

However a lot of these tests need access to a working Harvester, Openstack and Vmware cluster.

The integration tests can be setup by using the following environment variables to point the tests to a working environment to perform the actual vm migration tests

```shell
export GOVC_PASSWORD="vsphere-password"
export GOVC_USERNAME="vsphere-username"
export GOVC_URL="https://vcenter/sdk"
export GOVC_DATACENTER="vsphere-datacenter"
#The controller exposes the converted disks via a http endpoint and leverages the download capability of longhorn backing images
# the SVC address needs to be the address of node where integration tests are running and should be reachable from harvester cluster
export SVC_ADDRESS="address for node" 
export VM_NAME="vmware-export-test-vm-name"
export USE_EXISTING_CLUSTER=true
export OS_AUTH_URL="openstack/identity" #Keystone endpoint
export OS_PROJECT_NAME="openstack-project-name"
export OS_USER_DOMAIN_NAME="openstack-user-domain"
export OS_USERNAME="openstack-username"
export OS_PASSWORD="openstack-password"
export OS_VM_NAME="openstack-export-test-vm-name"
export OS_REGION_NAME="openstack-region"
export KUBECONFIG="kubeconfig-for-harvester-cluster"
```