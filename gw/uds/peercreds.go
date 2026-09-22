package uds

import (
	"fmt"
	"net"
	"os/user"
	"strconv"
)

// peerCreds is the connecting process's kernel-reported identity — uid
// (resolved to a username when the system knows one), gid, pid. This is
// attribution for the audit log, read from the kernel and unfakeable by the
// client; it is NOT authorization, so an unreadable credential degrades to
// "peer=?" rather than refusing the connection.
//
// Reading the credential is the operating system's business and lives in
// readPeerCred; what is reported and how it is spelled is shared, so every
// platform logs the same line.
func peerCreds(c net.Conn) string {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return "peer=?"
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return "peer=?"
	}
	var (
		uid, gid, pid uint32
		credErr       error
	)
	if err = raw.Control(func(fd uintptr) {
		uid, gid, pid, credErr = readPeerCred(fd)
	}); err != nil || credErr != nil {
		return "peer=?"
	}
	who := strconv.Itoa(int(uid))
	if u, lookErr := user.LookupId(who); lookErr == nil {
		who += "(" + u.Username + ")"
	}
	return fmt.Sprintf("uid=%s gid=%d pid=%d", who, gid, pid)
}
