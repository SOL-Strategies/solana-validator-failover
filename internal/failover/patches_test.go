package failover

import "testing"

func TestIdentityTransitionPatchURL(t *testing.T) {
	tests := []struct {
		name, version, release, want string
	}{
		{
			name:    "agave 4.2",
			version: "Agave 4.2.2",
			release: "v0.2.0",
			want:    "https://raw.githubusercontent.com/sol-strategies/solana-validator-failover/v0.2.0/patches/agave/admin-rpc-identity-transition-status/v4.2.x/0001-add-identity-transition-status-rpc.patch",
		},
		{
			name:    "agave 4.3 prerelease",
			version: "Agave 4.3.0-rc.1",
			release: "v0.2.0",
			want:    "https://raw.githubusercontent.com/sol-strategies/solana-validator-failover/v0.2.0/patches/agave/admin-rpc-identity-transition-status/v4.3.x/0001-add-identity-transition-status-rpc.patch",
		},
		{
			name:    "unknown version",
			version: "Agave 5.0.0",
			release: "v0.2.0",
			want:    "https://github.com/sol-strategies/solana-validator-failover/tree/v0.2.0/patches/agave/admin-rpc-identity-transition-status",
		},
		{
			name:    "development build",
			version: "Agave 4.2.2",
			release: "dev",
			want:    "https://raw.githubusercontent.com/sol-strategies/solana-validator-failover/main/patches/agave/admin-rpc-identity-transition-status/v4.2.x/0001-add-identity-transition-status-rpc.patch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IdentityTransitionPatchURL(test.version, test.release); got != test.want {
				t.Fatalf("IdentityTransitionPatchURL() = %q, want %q", got, test.want)
			}
		})
	}
}
