package qemu

import (
	"os/exec"
)

const defaultCommand = "qemu-img"

func ConvertVMDKtoRAW(source, target string) error {
	args := []string{"convert", "-f", "vmdk", "-O", "raw", source, target}
	cmd := exec.Command(defaultCommand, args...)
	return cmd.Run()
}

func ConvertQCOW2toRAW(source, target string) error {
	args := []string{"convert", "-f", "qcow2", "-O", "raw", source, target}
	cmd := exec.Command(defaultCommand, args...)
	return cmd.Run()
}

func createVMDK(path string, size string) error {
	args := []string{"create", "-f", "vmdk", path, size}
	cmd := exec.Command(defaultCommand, args...)
	return cmd.Run()
}
