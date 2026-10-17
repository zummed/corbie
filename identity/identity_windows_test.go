package identity

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// letOthersRead gives the file or directory an access list that lets the
// computer's users read it, and everything made in it.
func letOthersRead(t *testing.T, path string) {
	me, err := currentUser()
	if err != nil {
		t.Fatal(err)
	}
	users, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	if err != nil {
		t.Fatal(err)
	}
	entry := func(sid *windows.SID, access windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
		return windows.EXPLICIT_ACCESS{
			AccessPermissions: access,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		}
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{entry(me, windows.GENERIC_ALL), entry(users, windows.GENERIC_READ)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_id_identity_file_private_in_a_shared_directory(t *testing.T) {
	dir := t.TempDir()
	letOthersRead(t, dir)
	path := filepath.Join(dir, "runner", "identity")
	if err := party(t, "r1", Runner).Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("an identity saved where the computer's users can read: %v", err)
	}
}
