package util

import (
	"testing"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvesterutil "github.com/harvester/harvester/pkg/util"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/storage/v1"
)

func Test_GetBackendFromStorageClass(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc     string
		sc       *v1.StorageClass
		expected v1beta1.VMIBackend
	}{
		{
			desc: "Longhorn dataEngine V1",
			sc: &v1.StorageClass{
				Provisioner: harvesterutil.CSIProvisionerLonghorn,
				Parameters: map[string]string{
					"dataEngine": string(longhorn.DataEngineTypeV1),
				},
			},
			expected: v1beta1.VMIBackendBackingImage,
		},
		{
			desc: "Longhorn dataEngine V2",
			sc: &v1.StorageClass{
				Provisioner: harvesterutil.CSIProvisionerLonghorn,
				Parameters: map[string]string{
					"dataEngine": string(longhorn.DataEngineTypeV2),
				},
			},
			expected: v1beta1.VMIBackendCDI,
		},
		{
			desc: "LVM",
			sc: &v1.StorageClass{
				Provisioner: harvesterutil.CSIProvisionerLVM,
			},
			expected: v1beta1.VMIBackendCDI,
		},
	}
	for _, tc := range testCases {
		vmiBackend, err := GetBackendFromStorageClass(tc.sc)
		assert.NoError(err, "expect no error")
		assert.Equal(vmiBackend, tc.expected, tc.desc)
	}
}
