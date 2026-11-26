//go:build !linux && !windows

package agentactivity

import "net"

func isAntigravityProcess(uint32) bool                            { return false }
func antigravityOwnsPeer(uint32, *net.TCPAddr, *net.TCPAddr) bool { return false }
