package source

import (
	"k8s.io/utils/ptr"
	kubevirt "kubevirt.io/api/core/v1"
)

func VMSpecSetupUEFISettings(vmSpec *kubevirt.VirtualMachineSpec, secureBoot, tpm bool) {
	firmware := &kubevirt.Firmware{
		Bootloader: &kubevirt.Bootloader{
			EFI: &kubevirt.EFI{
				SecureBoot: ptr.To(false),
			},
		},
	}
	if secureBoot {
		firmware.Bootloader.EFI.SecureBoot = ptr.To(true)
	}
	vmSpec.Template.Spec.Domain.Firmware = firmware
	if tpm {
		vmSpec.Template.Spec.Domain.Devices.TPM = &kubevirt.TPMDevice{}
	}
	if secureBoot || tpm {
		vmSpec.Template.Spec.Domain.Features.SMM = &kubevirt.FeatureState{
			Enabled: ptr.To(true),
		}
	}
}
