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
apiVersion: source.harvesterhci.io/v1beta1
kind: Vmware
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
$ kubectl get vmware.source 
NAME      STATUS
vcsim   clusterReady
```

### ImportJob
The ImportJob crd provides a way for users to define the source VM and mapping to the actual source cluster to perform the VM export-import from.

A sample import job looks as follows:

```yaml
apiVersion: importjob.harvesterhci.io/v1beta1
kind: VirtualMachine
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
    kind: Vmware
    apiVersion: source.harvesterhci.io/v1beta1
```

This will trigger the controller to export the VM named "alpine-export-test" on the vmware source vcsim to be exported, processed and recreated into the harvester cluster

This can take a while based on the size of the virtual machine, but users should see `VirtualMachineImages` created for each disk in the defined virtual machine.

The list of items in `networkMapping` will define how the source network interfaces are mapped into the Harvester Networks.

If a match is not found, then each unmatched network inteface is attached to the default `managementNetwork`

Once the virtual machine has been imported successfully the object will reflect the status

```shell
$ kubectl get virtualmachine.importjob
NAME                 STATUS
alpine-export-test   virtualMachineRunning
```

