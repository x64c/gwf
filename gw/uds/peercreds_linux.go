//go:build linux

package uds

import "syscall"

// readPeerCred answers uid, gid and pid from SO_PEERCRED, which carries all
// three in one credential.
func readPeerCred(fd uintptr) (uid, gid, pid uint32, err error) {
	cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		return 0, 0, 0, err
	}
	return cred.Uid, cred.Gid, uint32(cred.Pid), nil
}
