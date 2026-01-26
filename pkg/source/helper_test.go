package source

import (
	"testing"

	harvesterutil "github.com/harvester/harvester/pkg/util"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"
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
		vmSpec := kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Features: &kubevirtv1.Features{},
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

func Test_NewVirtualMachineSpec(t *testing.T) {
	assert := require.New(t)
	testCases := []struct {
		desc               string
		config             VirtualMachineSpecConfig
		expectedName       string
		expectedCPUCores   uint32
		expectedCPUSockets uint32
		expectedMemory     string
	}{
		{
			desc: "Basic configuration",
			config: VirtualMachineSpecConfig{
				Name:     "basic-vm",
				Hardware: *NewHardware(4, 2, 8192, ""),
			},
			expectedCPUCores:   4,
			expectedCPUSockets: 2,
			expectedMemory:     "8192M",
		},
		{
			desc: "High CPU and memory configuration",
			config: VirtualMachineSpecConfig{
				Name:     "high-performance-vm",
				Hardware: *NewHardware(64, 32, 65536, ""),
			},
			expectedCPUCores:   64,
			expectedCPUSockets: 32,
			expectedMemory:     "65536M",
		},
		{
			desc: "Minimal hardware configuration",
			config: VirtualMachineSpecConfig{
				Name:     "minimal-vm",
				Hardware: *NewHardware(1, 1, 512, ""),
			},
			expectedCPUCores:   1,
			expectedCPUSockets: 1,
			expectedMemory:     "512M",
		},
	}

	for _, tc := range testCases {
		vmSpec := NewVirtualMachineSpec(tc.config)
		assert.Contains(vmSpec.Template.ObjectMeta.Labels, harvesterutil.LabelVMName, "expected VMName label to be present")
		assert.Equal(vmSpec.Template.ObjectMeta.Labels[harvesterutil.LabelVMName], tc.config.Name, "expected VMName label to match")
		assert.Equal(ptr.Deref(vmSpec.RunStrategy, ""), kubevirtv1.RunStrategyRerunOnFailure, "expected RunStrategy to match")
		assert.Equal(ptr.Deref(vmSpec.Template.Spec.EvictionStrategy, ""), kubevirtv1.EvictionStrategyLiveMigrateIfPossible, "expected EvictionStrategy to match")
		assert.Equal(vmSpec.Template.ObjectMeta.Labels["harvesterhci.io/vmName"], tc.config.Name, "expected VM Name to match")
		assert.Equal(vmSpec.Template.Spec.Domain.CPU.Cores, tc.expectedCPUCores, "expected CPU cores to match")
		assert.Equal(vmSpec.Template.Spec.Domain.CPU.Sockets, tc.expectedCPUSockets, "expected CPU cores to match")
		assert.Equal(vmSpec.Template.Spec.Domain.CPU.Threads, uint32(1), "expected CPU threads to match")
		assert.Equal(vmSpec.Template.Spec.Domain.Memory.Guest.String(), tc.expectedMemory, "expected memory to match")
		assert.Equal(vmSpec.Template.Spec.Domain.Resources.Limits.Memory().String(), tc.expectedMemory, "expected memory limit to match")
		assert.Equal(vmSpec.Template.Spec.Domain.Resources.Limits.Cpu().Value(), int64(tc.expectedCPUCores), "expected CPU limit to match")
		assert.Equal(vmSpec.Template.Spec.Domain.Features.ACPI.Enabled, ptr.To(true), "expected ACPI to be enabled")
	}
}
