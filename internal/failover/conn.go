package failover

import (
	"net"
	"time"
)

// basicPacketConn wraps a net.PacketConn and hides *net.UDPConn from quic-go.
// This prevents quic-go from using optimized OOB (recvmsg/sendmsg with IP_PKTINFO)
// paths that fail on certain virtual network interfaces (e.g. Tailscale/WireGuard tun).
// Instead, quic-go falls back to plain ReadFrom/WriteTo which works universally.
type basicPacketConn struct {
	conn net.PacketConn
}

func (c *basicPacketConn) ReadFrom(p []byte) (int, net.Addr, error) { return c.conn.ReadFrom(p) }
func (c *basicPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	return c.conn.WriteTo(p, addr)
}
func (c *basicPacketConn) Close() error                       { return c.conn.Close() }
func (c *basicPacketConn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *basicPacketConn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *basicPacketConn) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *basicPacketConn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

// newBasicPacketConn creates a UDP4 socket wrapped in basicPacketConn.
func newBasicPacketConn(addr string) (*basicPacketConn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return nil, err
	}
	return &basicPacketConn{conn: udpConn}, nil
}
