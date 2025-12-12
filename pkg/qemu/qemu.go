package qemu

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/sirupsen/logrus"
)

const defaultCommand = "qemu-wrapper.sh"

func ConvertVMDKtoRAW(source, target string) error {
	logrus.WithFields(logrus.Fields{
		"source": source,
		"target": target,
	}).Info("Converting VMDK image to RAW ...")
	args := []string{"convert", "-f", "vmdk", "-O", "raw", source, target}
	return runCommand(defaultCommand, args...)
}

func ConvertFromStdin(source io.Reader, target, format string) error {
	logrus.WithFields(logrus.Fields{
		"target": target,
		"format": format,
	}).Info("Converting image from stdin to RAW ...")
	// qemu-img convert -f <format> -O raw /dev/stdin <target>
	args := []string{"convert", "-f", format, "-O", "raw", "/dev/stdin", target}
	return runCommandWithStdin(defaultCommand, source, args...)
}

func createVMDK(path string, size string) error {
	args := []string{"create", "-f", "vmdk", path, size}
	return runCommand(defaultCommand, args...)
}

func runCommand(command string, args ...string) error {
	return runCommandWithStdin(command, nil, args...)
}

func runCommandWithStdin(command string, stdin io.Reader, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	if stdin != nil {
		cmd.Stdin = stdin
	}
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
