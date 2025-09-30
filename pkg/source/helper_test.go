package source

import (
	"testing"

	"github.com/stretchr/testify/require"
	kubevirt "kubevirt.io/api/core/v1"
)

func Test_vmSpecSetupUefiSettings(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc string
		fw   *Firmware
	}{
		{
			desc: "SecureBoot enabled, TPM disabled",
			fw:   NewFirmware(true, false, true),
		}, {
			desc: "SecureBoot disabled, TPM enabled",
			fw:   NewFirmware(true, true, false),
		}, {
			desc: "SecureBoot enabled, TPM enabled",
			fw:   NewFirmware(true, true, true),
		}, {
			desc: "SecureBoot disabled, TPM disabled",
			fw:   NewFirmware(true, false, false),
		}, {
			desc: "UEFI disabled",
			fw:   NewFirmware(false, true, true),
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
		ApplyFirmwareSettings(&vmSpec, tc.fw)
		if tc.fw.UEFI {
			if tc.fw.SecureBoot {
				assert.True(*vmSpec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot, "expected SecureBoot to be enabled")
			} else {
				assert.False(*vmSpec.Template.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot, "expected SecureBoot to be disabled")
			}
			if tc.fw.SecureBoot || tc.fw.TPM {
				assert.True(*vmSpec.Template.Spec.Domain.Features.SMM.Enabled, "expected SMM to be enabled")
			} else {
				assert.Nil(vmSpec.Template.Spec.Domain.Features.SMM, "expected SMM to be nil")
			}
		} else {
			assert.Nil(vmSpec.Template.Spec.Domain.Firmware, "expected 'Firmware' field to be nil")
			assert.Nil(vmSpec.Template.Spec.Domain.Devices.TPM, "expected 'TPM' field to be nil")
			assert.Nil(vmSpec.Template.Spec.Domain.Features.SMM, "expected 'SMM' field to be nil")
		}
	}
}
