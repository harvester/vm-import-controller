package source

import (
	"testing"

	"github.com/stretchr/testify/require"
	kubevirt "kubevirt.io/api/core/v1"
)

func Test_vmSpecSetupUefiSettings(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc       string
		secureBoot bool
		tpm        bool
	}{
		{
			desc:       "SecureBoot enabled, TPM disabled",
			secureBoot: true,
			tpm:        false,
		}, {
			desc:       "SecureBoot disabled, TPM enabled",
			secureBoot: false,
			tpm:        true,
		}, {
			desc:       "SecureBoot enabled, TPM enabled",
			secureBoot: true,
			tpm:        true,
		}, {
			desc:       "SecureBoot disabled, TPM disabled",
			secureBoot: false,
			tpm:        false,
		},
	}

	for _, tc := range testCases {
		vmSpec := kubevirt.VirtualMachineSpec{
			Template: &kubevirt.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirt.VirtualMachineInstanceSpec{
					Domain: kubevirt.DomainSpec{
						Features: &kubevirt.Features{},
					},
				},
			},
		}
		VMSpecSetupUEFISettings(&vmSpec, tc.secureBoot, tc.tpm)
		if tc.secureBoot {
			assert.True(*vmSpec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot, "expected SecureBoot to be enabled")
		} else {
			assert.False(*vmSpec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot, "expected SecureBoot to be disabled")
		}
		if tc.secureBoot || tc.tpm {
			assert.True(*vmSpec.Template.Spec.Domain.Features.SMM.Enabled, "expected SMM to be enabled")
		} else {
			assert.Nil(vmSpec.Template.Spec.Domain.Features.SMM, "expected SMM to be nil")
		}
	}
}
