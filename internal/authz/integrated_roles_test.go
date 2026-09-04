package authz

import "testing"

func TestIntegratedAdminModeratorUserOutcomes(t *testing.T) {
	outcomes := []struct {
		name      string
		role      Role
		allowed   []Capability
		forbidden []Capability
	}{
		{
			name: "Admin", role: RoleAdmin,
			allowed: []Capability{LibraryRead, MediaStream, MetadataWrite, AccountManage, SystemConfigure, JobManage, AuditRead},
		},
		{
			name: "Moderator", role: RoleModerator,
			allowed:   []Capability{LibraryRead, MediaStream, MetadataWrite, ActivitySelfWrite},
			forbidden: []Capability{AccountManage, SystemConfigure, JobManage, LibraryDestructive, MediaDownload},
		},
		{
			name: "User", role: RoleUser,
			allowed:   []Capability{LibraryRead, MediaStream, ActivitySelfWrite, PreferenceSelfWrite},
			forbidden: []Capability{MetadataWrite, AccountManage, SystemConfigure, JobManage, LibraryDestructive, MediaDownload},
		},
	}
	for _, outcome := range outcomes {
		t.Run(outcome.name, func(t *testing.T) {
			principal := Principal{UserID: "1", Role: outcome.role, Status: StatusActive}
			for _, capability := range outcome.allowed {
				if !principal.Allows(capability) {
					t.Errorf("%s lacks required integrated capability %s", outcome.role, capability)
				}
			}
			for _, capability := range outcome.forbidden {
				if principal.Allows(capability) {
					t.Errorf("%s obtained forbidden integrated capability %s", outcome.role, capability)
				}
			}
		})
	}
}
