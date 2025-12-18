package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	harvesterutil "github.com/harvester/harvester/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
)

type Firmware struct {
	UEFI       bool
	TPM        bool
	SecureBoot bool
}

func NewFirmware(uefi, tpm, secureBoot bool) *Firmware {
	return &Firmware{UEFI: uefi, TPM: tpm, SecureBoot: secureBoot}
}

type Hardware struct {
	NumCPU            uint32 // The type is adapted to KubeVirt CPU
	NumCoresPerSocket uint32 // The type is adapted to KubeVirt CPU
	MemoryMB          int64
	CPUModel          string
}

func NewHardware(numCPU, numCoresPerSocket uint32, memoryMB int64, cpuModel string) *Hardware {
	return &Hardware{NumCPU: numCPU, NumCoresPerSocket: numCoresPerSocket, MemoryMB: memoryMB, CPUModel: cpuModel}
}

type VirtualMachineSpecConfig struct {
	Name     string
	Hardware Hardware
}

func NewVirtualMachineSpec(cfg VirtualMachineSpecConfig) *kubevirtv1.VirtualMachineSpec {
	return &kubevirtv1.VirtualMachineSpec{
		RunStrategy: ptr.To(kubevirtv1.RunStrategyRerunOnFailure),
		Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					harvesterutil.LabelVMName: cfg.Name,
				},
			},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				EvictionStrategy: ptr.To(kubevirtv1.EvictionStrategyLiveMigrateIfPossible),
				Domain: kubevirtv1.DomainSpec{
					CPU: &kubevirtv1.CPU{
						Cores:   cfg.Hardware.NumCPU,
						Sockets: cfg.Hardware.NumCoresPerSocket,
						Threads: 1,
						Model:   cfg.Hardware.CPUModel,
					},
					Memory: &kubevirtv1.Memory{
						Guest: ptr.To(resource.MustParse(fmt.Sprintf("%dM", cfg.Hardware.MemoryMB))),
					},
					Resources: kubevirtv1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dM", cfg.Hardware.MemoryMB)),
							corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", cfg.Hardware.NumCPU)),
						},
					},
					Features: &kubevirtv1.Features{
						ACPI: kubevirtv1.FeatureState{
							Enabled: ptr.To(true),
						},
					},
				},
			},
		},
	}
}

func ApplyFirmwareSettings(vmSpec *kubevirtv1.VirtualMachineSpec, fw *Firmware) {
	if !fw.UEFI {
		return
	}

	firmware := &kubevirtv1.Firmware{
		Bootloader: &kubevirtv1.Bootloader{
			EFI: &kubevirtv1.EFI{
				SecureBoot: ptr.To(false),
			},
		},
	}

	if fw.SecureBoot {
		firmware.Bootloader.EFI.SecureBoot = ptr.To(true)
	}

	vmSpec.Template.Spec.Domain.Firmware = firmware

	if fw.TPM {
		vmSpec.Template.Spec.Domain.Devices.TPM = &kubevirtv1.TPMDevice{}
	}
	if fw.SecureBoot || fw.TPM {
		vmSpec.Template.Spec.Domain.Features.SMM = &kubevirtv1.FeatureState{
			Enabled: ptr.To(true),
		}
	}
}

// RemoveTempImageFiles removes temporary image files used during migration.
// Not existing files are ignored. All occurring errors are aggregated and returned.
func RemoveTempImageFiles(dis []migration.DiskInfo) error {
	var errs []error

	for _, di := range dis {
		err := os.Remove(filepath.Join(server.TempDir(), di.Name))
		if err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("failed to remove image file: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
