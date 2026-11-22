package main

import "nook/core/netguard"

func tunnelsUp() bool { return len(netguard.Carrying(netguard.Tunnels())) > 0 }
