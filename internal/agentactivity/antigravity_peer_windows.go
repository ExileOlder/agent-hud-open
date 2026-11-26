package agentactivity

import (
	"bytes"
	"encoding/binary"
	"math/bits"
	"net"
	"syscall"
	"unsafe"
)

var (
	getAntigravityTCPTable = syscall.NewLazyDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")
	queryAntigravityImage  = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
)

func isAntigravityProcess(pid uint32) bool {
	h, err := syscall.OpenProcess(0x1000, false, pid) // PROCESS_QUERY_LIMITED_INFORMATION
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var path [32768]uint16
	n := uint32(len(path))
	ok, _, _ := queryAntigravityImage.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&n)))
	return ok != 0 && isAntigravityExecutable(syscall.UTF16ToString(path[:n]))
}

func antigravityOwnsPeer(pid uint32, local, remote *net.TCPAddr) bool {
	// A tcp4 client can reach a dual-stack listener, whose accepted
	// socket may be reported in the IPv6 table.
	return windowsPeerInTable(2, 24, pid, local, remote) || windowsPeerInTable(23, 56, pid, local, remote)
}

func windowsPeerInTable(family, rowSize int, pid uint32, local, remote *net.TCPAddr) bool {
	var size uint32
	const connections = 4 // TCP_TABLE_OWNER_PID_CONNECTIONS
	result, _, _ := getAntigravityTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, uintptr(family), connections, 0)
	if result != 122 || size < 4 { // ERROR_INSUFFICIENT_BUFFER
		return false
	}
	table := make([]byte, size)
	result, _, _ = getAntigravityTCPTable.Call(uintptr(unsafe.Pointer(&table[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), connections, 0)
	if result != 0 {
		return false
	}
	u32 := binary.LittleEndian.Uint32
	serverPort, clientPort := bits.ReverseBytes16(uint16(remote.Port)), bits.ReverseBytes16(uint16(local.Port))
	count := int(u32(table))
	for i := 0; i < count && 4+(i+1)*rowSize <= int(size); i++ {
		row := table[4+i*rowSize:]
		if family == 2 {
			if u32(row) == 5 && bytes.Equal(row[4:8], remote.IP.To4()) && uint16(u32(row[8:])) == serverPort &&
				bytes.Equal(row[12:16], local.IP.To4()) && uint16(u32(row[16:])) == clientPort && u32(row[20:]) == pid {
				return true
			}
		} else if u32(row[48:]) == 5 && bytes.Equal(row[:16], remote.IP.To16()) && uint16(u32(row[20:])) == serverPort &&
			bytes.Equal(row[24:40], local.IP.To16()) && uint16(u32(row[44:])) == clientPort && u32(row[52:]) == pid {
			return true
		}
	}
	return false
}
