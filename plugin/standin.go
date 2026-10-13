package plugin

import (
	"context"
	"net"
)

// StandIn puts a stand-in in place of the network for every job of this
// process: names gives the addresses a name has, and connect opens a
// connection to an address. Every check is made as on the real network,
// on the name and on the addresses given. It is for tests that run
// scripts as they are written, against servers of the tests' own.
func StandIn(names func(ctx context.Context, host string) ([]string, error), connect func(ctx context.Context, address string) (net.Conn, error)) {
	lookup, dial = names, connect
}
