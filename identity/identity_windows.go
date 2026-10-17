package identity

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows a file's mode does not say who can read it; its access list
// does. SYSTEM and Administrators can read every file, as root can
// elsewhere, so an entry for them gives nothing away.

// makePrivate gives the file an access list with one entry, for the user
// Corbie runs as, and takes no entries from its directory.
func makePrivate(path string) error {
	me, err := currentUser()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(me),
		},
	}}, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

// checkPrivate refuses a file that anyone but the user Corbie runs as,
// SYSTEM and Administrators can read.
func checkPrivate(path string, _ fs.FileInfo) error {
	me, err := currentUser()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	// A file with no access list can be read by everyone.
	acl, _, err := sd.DACL()
	if err != nil && !errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) {
		return err
	}
	refused := fmt.Errorf("the identity file %q can be read by others; its key is no longer its owner's alone. If that is not so, run: icacls \"%s\" /inheritance:r /grant:r *%s:F",
		path, path, me)
	if acl == nil {
		return refused
	}
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 ||
			ace.Mask&(windows.FILE_READ_DATA|windows.GENERIC_READ|windows.GENERIC_ALL) == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !slices.ContainsFunc([]*windows.SID{me, system, admins}, sid.Equals) {
			return refused
		}
	}
	return nil
}

// currentUser gives the user Corbie runs as.
func currentUser() (*windows.SID, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return u.User.Sid, nil
}
