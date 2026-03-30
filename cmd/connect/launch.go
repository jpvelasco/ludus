package connect

// buildLaunchArgs returns the argument list for launching the game client.
// The server address is passed as a travel URL (first positional arg)
// so UE5 frontends like Lyra connect directly on startup.
func buildLaunchArgs(connectAddr string) []string {
	return []string{connectAddr, "-game", "-log"}
}
