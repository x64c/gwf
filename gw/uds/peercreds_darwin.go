//go:build darwin

package uds

import (
	"errors"

	"golang.org/x/sys/unix"
)

// readPeerCred answers uid, gid and pid from two socket options: LOCAL_PEERCRED
// carries the uid and the peer's group vector, whose first entry is the
// effective gid, and LOCAL_PEERPID carries the pid.
func readPeerCred(fd uintptr) (uid, gid, pid uint32, err error) {
	xucred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return 0, 0, 0, err
	}
	if xucred.Ngroups < 1 {
		return 0, 0, 0, errors.New("uds: peer credential carries no group")
	}
	peerPID, err := unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	if err != nil {
		return 0, 0, 0, err
	}
	return xucred.Uid, xucred.Groups[0], uint32(peerPID), nil
}
