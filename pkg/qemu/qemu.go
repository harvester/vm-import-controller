package qemu

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/sirupsen/logrus"
)

const defaultCommand = "qemu-wrapper.sh"

func ConvertToRAW(source, target string, format string) error {
	logrus.WithFields(logrus.Fields{
		"source": source,
		"target": target,
		"format": format,
	}).Info("Converting VMDK image ...")
	args := []string{"convert", "-f", format, "-O", "raw", source, target}
	return runCommand(defaultCommand, args...)
}

func createVMDK(path string, size string) error {
	args := []string{"create", "-f", "vmdk", path, size}
	return runCommand(defaultCommand, args...)
}

func runCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error in command start: %v", err)
	}

	errOut, _ := io.ReadAll(stderr)
	out, err := io.ReadAll(stdout)
	if err != nil {
		return fmt.Errorf("error reading command output: %v", err)
	}
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("error in command: %s, %s", command, errOut)
	}
	logrus.Debugf("image command complete: %v", string(out))
	return nil
}
