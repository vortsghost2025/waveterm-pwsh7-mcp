package aiusechat

import "testing"

func TestCheckCommandScopedAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "allows docker inspect with pipe",
			command: `docker inspect federation-game-frontend-1 | grep -A 10 Mounts`,
		},
		{
			name:    "allows scoped set-location build",
			command: `Set-Location S:\waveterm; npm run build`,
		},
		{
			name:    "allows scoped ssh command",
			command: `ssh -i "~/.ssh/id_ed25519" root@100.75.95.23 "ls -la /docker/federation-game/frontend/"`,
		},
		{
			name:    "allows generic docker ps",
			command: `docker ps`,
		},
		{
			name:    "allows generic docker inspect",
			command: `docker inspect my-container`,
		},
		{
			name:    "allows ssh docker ps on arbitrary host",
			command: `ssh root@1.2.3.4 "docker ps"`,
		},
		{
			name:    "allows ssh alias with service status",
			command: `ssh my-vps "systemctl status nginx"`,
		},
		{
			name:    "rejects unallowlisted metacharacters",
			command: `echo hello && whoami`,
			wantErr: true,
		},
		{
			name:    "rejects destructive remote docker command",
			command: `ssh root@1.2.3.4 "docker rm my-container"`,
			wantErr: true,
		},
		{
			name:    "rejects set-location outside scoped paths",
			command: `Set-Location C:\Users\seand; npm run build`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCommand(tc.command)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.command)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.command, err)
			}
		})
	}
}
