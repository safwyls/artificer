package palworld

import "github.com/safwyls/sampo/core/api"

// ProvisionProfile is Palworld's provisioning knowledge: the UDP game
// port plus the REST and RCON admin transports — four published ports,
// all distinct (drift ledger, seam 4's trio form).
func ProvisionProfile() *api.ProvisionProfile {
	return &api.ProvisionProfile{
		AgentName:       "palagent",
		ImageRepo:       "ghcr.io/safwyls/palagent",
		EnvPrefix:       "PALAGENT",
		DefaultGamePort: Definition.DefaultGamePort,
		MountPath:       "/palworld",
		SlugFallback:    "palworld",
		StackHeadline:   "Palworld server supervised by palagent",
		StackNotes: "# On first boot the agent installs the game via SteamCMD — watch\n" +
			"# progress from the server's dashboard card — and starts it already\n" +
			"# wired for REST/RCON.\n",
		GamePortComment: "game",
		AdminPorts: []api.AdminPort{
			{Key: "rest", Container: 8212, Default: 8212, Comment: "REST (dashboard)"},
			{Key: "rcon", Container: 25575, Default: 25575, Comment: "RCON (dashboard fallback)"},
		},
	}
}
