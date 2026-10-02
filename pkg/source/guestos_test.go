package source

import (
	"testing"

	"github.com/vmware/govmomi/vim25/types"
)

func TestGuestOsIdToOsType(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected string
	}{
		{name: "modern windows", id: string(types.VirtualMachineGuestOsIdentifierWindows11_64Guest), expected: OsWindows},
		{name: "windows server 2003", id: string(types.VirtualMachineGuestOsIdentifierWinNetStandardGuest), expected: OsWindows},
		{name: "windows xp", id: string(types.VirtualMachineGuestOsIdentifierWinXPProGuest), expected: OsWindows},
		{name: "windows 98", id: string(types.VirtualMachineGuestOsIdentifierWin98Guest), expected: OsWindows},
		{name: "sles", id: string(types.VirtualMachineGuestOsIdentifierSles15_64Guest), expected: OsSLES},
		{name: "plain suse id also maps to sles", id: string(types.VirtualMachineGuestOsIdentifierSuse64Guest), expected: OsSLES},
		{name: "opensuse", id: string(types.VirtualMachineGuestOsIdentifierOpensuse64Guest), expected: OsOpenSUSE},
		{name: "debian", id: string(types.VirtualMachineGuestOsIdentifierDebian12_64Guest), expected: OsDebian},
		{name: "fedora", id: string(types.VirtualMachineGuestOsIdentifierFedora64Guest), expected: OsFedora},
		{name: "oracle linux", id: string(types.VirtualMachineGuestOsIdentifierOracleLinux8_64Guest), expected: OsOracle},
		{name: "rhel", id: string(types.VirtualMachineGuestOsIdentifierRhel9_64Guest), expected: OsRedHat},
		{name: "redhat", id: string(types.VirtualMachineGuestOsIdentifierRedhatGuest), expected: OsRedHat},
		{name: "ubuntu", id: string(types.VirtualMachineGuestOsIdentifierUbuntu64Guest), expected: OsUbuntu},
		{name: "centos maps to harvester otherLinux entry", id: string(types.VirtualMachineGuestOsIdentifierCentos7_64Guest), expected: OsOtherLinux},
		{name: "generic linux family", id: string(types.VirtualMachineGuestOsIdentifierOther7xLinux64Guest), expected: OsLinux},
		{name: "amazon linux", id: string(types.VirtualMachineGuestOsIdentifierAmazonlinux2_64Guest), expected: OsLinux},
		{name: "rocky linux", id: string(types.VirtualMachineGuestOsIdentifierRockylinux_64Guest), expected: OsLinux},
		{name: "bare rocky, e.g. openstack's os_distro value", id: "rocky", expected: OsLinux},
		{name: "bare arch, e.g. openstack's os_distro value", id: "arch", expected: OsLinux},
		{name: "bare sled, e.g. openstack's os_distro value", id: "sled", expected: OsSLES},
		{name: "gentoo, e.g. openstack's os_distro value, has no vmware guest id equivalent", id: "gentoo", expected: OsGentoo},
		{name: "darwin is not windows", id: string(types.VirtualMachineGuestOsIdentifierDarwin19_64Guest), expected: OsUnknown},
		{name: "freebsd is unknown", id: string(types.VirtualMachineGuestOsIdentifierFreebsd64Guest), expected: OsUnknown},
		{name: "unknown", id: string(types.VirtualMachineGuestOsIdentifierOtherGuest), expected: OsUnknown},
		{name: "empty", id: "", expected: OsUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GuestOsIdToOsType(tt.id); got != tt.expected {
				t.Fatalf("expected %q got %q", tt.expected, got)
			}
		})
	}
}

func TestGuestOsNameToOsType(t *testing.T) {
	tests := []struct {
		name      string
		guestName string
		expected  string
	}{
		{name: "windows", guestName: "Microsoft Windows Server 2019", expected: OsWindows},
		{name: "gentoo has no vmware guest id, only detected via free text", guestName: "Gentoo", expected: OsGentoo},
		{name: "sles free text", guestName: "SUSE Linux Enterprise Server 15", expected: OsSLES},
		{name: "opensuse free text is not sles", guestName: "openSUSE Leap 15.5", expected: OsOpenSUSE},
		{name: "red hat with a space", guestName: "Red Hat Enterprise Linux 8 (64-bit)", expected: OsRedHat},
		{name: "centos free text maps to harvester otherLinux entry", guestName: "CentOS 7 (64-bit)", expected: OsOtherLinux},
		{name: "generic linux distro fallback", guestName: "Ubuntu Linux (64-bit)", expected: OsUbuntu},
		{name: "unclassified linux falls back to the generic entry", guestName: "Some Linux 5 (64-bit)", expected: OsLinux},
		{name: "darwin free text is not windows", guestName: "Darwin 19.0", expected: OsUnknown},
		{name: "unknown", guestName: "Other (32-bit)", expected: OsUnknown},
		{name: "empty", guestName: "", expected: OsUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GuestOsNameToOsType(tt.guestName); got != tt.expected {
				t.Fatalf("expected %q got %q", tt.expected, got)
			}
		})
	}
}
