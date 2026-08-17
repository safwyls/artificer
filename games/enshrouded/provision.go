package enshrouded

import "github.com/safwyls/artificer/core/api"

// ProvisionProfile is Enshrouded's provisioning knowledge for the
// Raise-a-server wizard and the Anvil adapter — the values that used to
// be flametender's hardcodes (drift ledger, seam 4).
func ProvisionProfile() *api.ProvisionProfile {
	return &api.ProvisionProfile{
		AgentName:       "flameagent",
		ImageRepo:       "ghcr.io/safwyls/flameagent",
		EnvPrefix:       "FLAMEAGENT",
		DefaultGamePort: DefaultQueryPort,
		MountPath:       "/enshrouded",
		SlugFallback:    "enshrouded",
		StackHeadline:   "Enshrouded server supervised by flameagent",
		StackNotes: "# On first boot the agent installs the game via SteamCMD — watch\n" +
			"# progress from the server's dashboard card — seeds\n" +
			"# enshrouded_server.json, and starts the server under Wine.\n",
		GamePortComment: "game + Steam query (Enshrouded's single UDP port)",
	}
}
