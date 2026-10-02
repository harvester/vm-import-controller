package source

import "strings"

// The OS values as offered by the Harvester UI's "OS Type" dropdown.
const (
	OsWindows    = "windows"
	OsLinux      = "linux"
	OsSLES       = "SLEs"
	OsDebian     = "debian"
	OsFedora     = "fedora"
	OsGentoo     = "gentoo"
	OsOracle     = "oracle"
	OsRedHat     = "redhat"
	OsOpenSUSE   = "openSUSE"
	OsUbuntu     = "ubuntu"
	OsOtherLinux = "otherLinux"
	OsUnknown    = ""
)

// guestOsMapping maps a keyword to the OS value it identifies.
type guestOsMapping struct {
	match string
	os    string
}

// guestOsIdPrefixes classifies structured/enumerated guest OS identifiers
// by prefix, i.e. values chosen from a fixed, recognized vocabulary at VM
// or image creation time, as opposed to arbitrary free text. Most of this
// vocabulary originates from VMware's vim25/types.VirtualMachineGuestOsIdentifier
// (see govmomi's vim25/types/enum.go), which names identifiers after the
// distribution they represent, e.g. "ubuntu64Guest", "rhel9_64Guest",
// "opensuse64Guest". The same vocabulary is reused by VMware-exported OVF
// files (the OperatingSystemSection's "vmw:osType" attribute) and largely
// overlaps with OpenStack Glance's standardized "os_distro" image property
// (e.g. "ubuntu", "rhel", "rocky" - see
// https://docs.openstack.org/glance/latest/admin/useful-image-properties.html),
// whose recognized values are themselves prefixes of the VMware spelling.
// A handful of entries (e.g. "gentoo", "arch", "sled") are recognized
// "os_distro" values with no VMware guest OS identifier at all. They're
// included here anyway, rather than only in guestOsNameKeywords below,
// because "os_distro" is still a controlled vocabulary, not free text.
// That's why this single table is shared between the vmware, ova and
// openstack importers rather than duplicated per package.
//
// Plain string prefixes are used deliberately, instead of the typed
// types.VirtualMachineGuestOsIdentifierXxx constants: VMware keeps naming new
// releases after the same prefix (e.g. a future "debian14_64Guest"), so
// prefix matching detects them automatically even with an older govmomi
// version that doesn't define the constant yet. Matching against the
// constants instead would leave every new identifier unrecognised until we
// upgrade govmomi and add it here by hand.
var guestOsIdPrefixes = []guestOsMapping{
	{"opensuse", OsOpenSUSE},
	{"sles", OsSLES},
	{"sled", OsSLES},
	{"suse", OsSLES},
	{"debian", OsDebian},
	{"fedora", OsFedora},
	{"oraclelinux", OsOracle},
	{"redhat", OsRedHat},
	{"rhel", OsRedHat},
	{"ubuntu", OsUbuntu},
	{"centos", OsOtherLinux},
	{"almalinux", OsLinux},
	{"amazonlinux", OsLinux},
	{"arch", OsLinux},
	{"asianux", OsLinux},
	{"coreos", OsLinux},
	{"fusionos", OsLinux},
	{"genericlinux", OsLinux},
	{"gentoo", OsGentoo},
	{"kylinlinux", OsLinux},
	{"mandrake", OsLinux},
	{"mandriva", OsLinux},
	{"miraclelinux", OsLinux},
	{"nld9", OsLinux},
	{"oes", OsLinux},
	{"pardus", OsLinux},
	{"prolinux", OsLinux},
	{"rocky", OsLinux},
	{"turbolinux", OsLinux},
	{"vmwarephoton", OsLinux},
}

// guestOsNameKeywords are a best-effort fallback over free text OS name
// fields (e.g. VMware's GuestFullName/AlternateGuestName, or an OVF
// OperatingSystemSection's Description), used when guestOsIdPrefixes yields
// no match, e.g. for "otherGuest"/custom guest IDs, or for distros that have
// no dedicated guest OS identifier at all, such as Gentoo. Unlike
// guestOsIdPrefixes, "win" alone is not matched here: free text isn't bound
// to VMware's fixed identifier vocabulary and "win" would false-positive on
// e.g. "Darwin". The generic "linux" entry is kept last so more specific
// distro keywords are tried first.
var guestOsNameKeywords = []guestOsMapping{
	{"windows", OsWindows},
	{"gentoo", OsGentoo},
	{"opensuse", OsOpenSUSE},
	{"sles", OsSLES},
	{"suse", OsSLES},
	{"debian", OsDebian},
	{"fedora", OsFedora},
	{"oracle", OsOracle},
	{"red hat", OsRedHat},
	{"redhat", OsRedHat},
	{"rhel", OsRedHat},
	{"ubuntu", OsUbuntu},
	{"centos", OsOtherLinux},
	{"linux", OsLinux},
}

// GuestOsIdToOsType classifies a VMware guest OS identifier value (see
// vim25/types.VirtualMachineGuestOsIdentifier) using guestOsIdPrefixes.
// Every Windows identifier starts with "win" (e.g. "windows9_64Guest",
// "winXPProGuest", "winNetStandardGuest" for Windows Server 2003), so that
// case is handled directly.
func GuestOsIdToOsType(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return OsUnknown
	}

	if strings.HasPrefix(id, "win") {
		return OsWindows
	}

	for _, prefix := range guestOsIdPrefixes {
		if strings.HasPrefix(id, prefix.match) {
			return prefix.os
		}
	}

	// Covers other24xLinuxGuest, other3xLinux64Guest, ... otherLinuxGuest,
	// otherLinux64Guest: some unspecified Linux flavor VMware couldn't name
	// explicitly. This maps to the generic OsLinux value, not to Harvester's
	// dedicated "Other Linux" entry.
	if strings.HasPrefix(id, "other") && strings.Contains(id, "linux") {
		return OsLinux
	}

	return OsUnknown
}

// GuestOsNameToOsType classifies a free text OS name using guestOsNameKeywords.
func GuestOsNameToOsType(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return OsUnknown
	}

	for _, keyword := range guestOsNameKeywords {
		if strings.Contains(name, keyword.match) {
			return keyword.os
		}
	}

	return OsUnknown
}
