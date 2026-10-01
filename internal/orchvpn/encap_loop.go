package orchvpn

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"
)

type encapReadLogMessages struct {
	deadline string
	read     string
}

func runEncapReadLoop(ctx context.Context, log *slog.Logger, pc net.PacketConn, obs EncapObserver, messages encapReadLogMessages) {
	buf := make([]byte, 65535)
	for {
		if ctx.Err() != nil {
			return
		}
		if !setEncapReadDeadline(log, pc, messages.deadline) {
			continue
		}
		n, remote, ok := readEncapPacket(ctx, log, pc, buf, messages.read)
		if !ok {
			continue
		}
		if n > 0 {
			HandleEncapUDP(pc, remote, buf[:n], obs)
		}
	}
}

func setEncapReadDeadline(log *slog.Logger, pc net.PacketConn, message string) bool {
	if err := pc.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		log.Debug(message, "error", err)
		return false
	}
	return true
}

func readEncapPacket(ctx context.Context, log *slog.Logger, pc net.PacketConn, buf []byte, message string) (int, net.Addr, bool) {
	n, remote, err := pc.ReadFrom(buf)
	if err == nil {
		return n, remote, true
	}
	handleEncapReadError(ctx, log, err, message)
	return 0, nil, false
}

func handleEncapReadError(ctx context.Context, log *slog.Logger, err error, message string) {
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr != nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	log.Warn(message, "error", err)
}
