package database

import "strings"

const DefaultNetwork = "libera"

func normalizeNetwork(network string) string {
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		return DefaultNetwork
	}
	return network
}
