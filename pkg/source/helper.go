package source

import (
	"k8s.io/utils/pointer"
	kubevirt "kubevirt.io/api/core/v1"
)

func VMSpecSetupUEFISettings(vmSpec *kubevirt.VirtualMachineSpec, secureBoot, tpm bool) {
	firmware := &kubevirt.Firmware{
		Bootloader: &kubevirt.Bootloader{
			EFI: &kubevirt.EFI{
				SecureBoot: pointer.Bool(false),
			},
		},
	}
	if secureBoot {
		firmware.Bootloader.EFI.SecureBoot = pointer.Bool(true)
	}
	vmSpec.Template.Spec.Domain.Firmware = firmware
	if tpm {
		vmSpec.Template.Spec.Domain.Devices.TPM = &kubevirt.TPMDevice{}
	}
	if secureBoot || tpm {
		vmSpec.Template.Spec.Domain.Features.SMM = &kubevirt.FeatureState{
			Enabled: pointer.Bool(true),
		}
	}
}
