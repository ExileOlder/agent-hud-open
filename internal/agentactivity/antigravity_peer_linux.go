package agentactivity

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
)

func isAntigravityProcess(pid uint32) bool {
	dir := fmt.Sprintf("/proc/%d", pid)
	st, err := os.Stat(dir)
	if err != nil || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return false
	}
	exe, err := os.Readlink(dir + "/exe")
	return err == nil && isAntigravityExecutable(exe)
}

func procTCPAddress(a *net.TCPAddr, ipv6 bool) string {
	ip := a.IP.To4()
	prefix := ""
	if ipv6 {
		prefix = "0000000000000000FFFF0000"
	}
	return fmt.Sprintf("%s%02X%02X%02X%02X:%04X", prefix, ip[3], ip[2], ip[1], ip[0], a.Port)
}

func antigravityOwnsPeer(pid uint32, local, remote *net.TCPAddr) bool {
	dir := fmt.Sprintf("/proc/%d/fd", pid)
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, ipv6 := range []bool{false, true} {
		path := "/proc/net/tcp"
		if ipv6 {
			path += "6"
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		server, client := procTCPAddress(remote, ipv6), procTCPAddress(local, ipv6)
		scan := bufio.NewScanner(f)
		inode := ""
		for scan.Scan() {
			fields := strings.Fields(scan.Text())
			if len(fields) >= 10 && fields[1] == server && fields[2] == client && fields[3] == "01" {
				inode = fields[9]
				break
			}
		}
		_ = f.Close()
		if inode == "" || inode == "0" {
			continue
		}
		want := "socket:[" + inode + "]"
		for _, file := range files {
			if target, err := os.Readlink(dir + "/" + file.Name()); err == nil && target == want {
				return true
			}
		}
	}
	return false
}
