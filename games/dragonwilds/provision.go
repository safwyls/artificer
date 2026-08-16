package dragonwilds

import "github.com/safwyls/sampo/core/api"

// ProvisionProfile is Dragonwilds' provisioning knowledge for the
// Raise-a-server wizard and the Ilmari adapter: the UDP port pair, the
// required owner id, and the wkagent image family (drift ledger, seam 4).
func ProvisionProfile() *api.ProvisionProfile {
	return &api.ProvisionProfile{
		AgentName:       "wkagent",
		ImageRepo:       "ghcr.io/safwyls/wkagent",
		EnvPrefix:       "WKAGENT",
		DefaultGamePort: Definition.DefaultGamePort,
		MountPath:       "/dragonwilds",
		SlugFallback:    "dragonwilds",
		StackHeadline:   "Dragonwilds server supervised by wkagent",
		StackNotes: "# On first boot the agent installs the game via SteamCMD — watch\n" +
			"# progress from the server's dashboard card — seeds\n" +
			"# DedicatedServer.ini, and starts the native Linux build.\n",
		GamePortComment: "game port (first of the UDP pair)",
		// The game binds GamePort and GamePort+1; getting the pair wrong
		// is the silent kind of broken — the server boots and nobody can
		// join.
		GamePortCount:   2,
		OwnerIDRequired: true,
		OwnerIDHelp:     `in-game: Settings, bottom-left "My Player ID"`,
	}
}
