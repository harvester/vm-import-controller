package util

import (
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterutil "github.com/harvester/harvester/pkg/util"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/storage/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

// GetStorageClassByName retrieves the storage class by its name from the provided cache.
func GetStorageClassByName(scName string, scCache ctlstoragev1.StorageClassCache) (*v1.StorageClass, error) {
	sc, err := scCache.Get(scName)
	if err != nil {
		logrus.Errorf("failed to get storage class '%s': %v", scName, err)
		return nil, err
	}
	return sc, nil
}

// GetBackendFromStorageClassName returns the VMIBackend type based on the storage class name.
func GetBackendFromStorageClassName(scName string, scCache ctlstoragev1.StorageClassCache) (v1beta1.VMIBackend, error) {
	sc, err := GetStorageClassByName(scName, scCache)
	if err != nil {
		return "", err
	}
	return getBackendFromStorageClass(sc)
}

// getBackendFromStorageClass returns the VMIBackend type based on the storage class.
func getBackendFromStorageClass(sc *v1.StorageClass) (v1beta1.VMIBackend, error) {
	vmiBackend := harvesterv1beta1.VMIBackendCDI
	if sc.Provisioner == harvesterutil.CSIProvisionerLonghorn {
		if dataEngine, ok := sc.Parameters["dataEngine"]; ok {
			if dataEngine == string(longhorn.DataEngineTypeV1) {
				vmiBackend = harvesterv1beta1.VMIBackendBackingImage
			}
		}
	}
	return vmiBackend, nil
}
