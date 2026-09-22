package failover

import "testing"

func TestIdentityTransitionPatchURL(t *testing.T) {
	tests := []struct {
		name, version, release, want string
	}{
		{
			name:    "agave 4.3.0",
			version: "Agave 4.3.0",
			release: "v0.2.0",
			want:    "https://github.com/sol-strategies/solana-validator-failover/tree/v0.2.0/patches",
		},
		{
			name:    "unknown version",
			version: "Agave 5.0.0",
			release: "v0.2.0",
			want:    "https://github.com/sol-strategies/solana-validator-failover/tree/v0.2.0/patches",
		},
		{
			name:    "development build",
			version: "Agave 4.3.0",
			release: "dev",
			want:    "https://github.com/sol-strategies/solana-validator-failover/tree/main/patches",
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
