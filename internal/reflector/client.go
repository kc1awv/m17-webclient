package reflector

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	log "github.com/kc1awv/m17-webclient/internal/logger"
	"github.com/kc1awv/m17-webclient/internal/m17"
)

type Event int

const (
	EventNACK Event = iota
)

type ReflectorClient struct {
	UDPConn    *net.UDPConn
	RemoteAddr *net.UDPAddr
	Callsign   string
	Module     byte
	Designator string
	connected  bool
	lastPing   time.Time
	ctx        context.Context
	cancel     context.CancelFunc

	Packets   chan []byte
	Events    chan Event
	closeOnce sync.Once
}

func NewReflectorClient(ctx context.Context, reflectorAddr, callsign string, module byte) (*ReflectorClient, error) {
	remote, err := net.ResolveUDPAddr("udp", reflectorAddr)
	if err != nil {
		return nil, err
	}

	network := "udp4"
	if remote.IP.To4() == nil {
		network = "udp6"
	}

	local := &net.UDPAddr{Port: 0}

	conn, err := net.ListenUDP(network, local)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	client := &ReflectorClient{
		UDPConn:    conn,
		RemoteAddr: remote,
		Callsign:   callsign,
		Module:     module,
		lastPing:   time.Now(),
		ctx:        ctx,
		cancel:     cancel,
		Packets:    make(chan []byte, 100),
		Events:     make(chan Event, 10),
	}

	if err := client.sendControl(func() ([]byte, error) {
		return m17.BuildCONN(client.Callsign, client.Module)
	}); err != nil {
		log.Error("Error sending CONN", "err", err, "reflector", client.Designator)
		conn.Close()
		cancel()
		return nil, err
	}

	go client.listen()
	go client.monitorPing()

	return client, nil
}

func NewTestClient(ctx context.Context, conn *net.UDPConn, remote *net.UDPAddr, callsign string, module byte, designator string, packets chan []byte, events chan Event) *ReflectorClient {
	ctx, cancel := context.WithCancel(ctx)
	if packets == nil {
		packets = make(chan []byte, 100)
	}
	if events == nil {
		events = make(chan Event, 10)
	}
	return &ReflectorClient{
		UDPConn:    conn,
		RemoteAddr: remote,
		Callsign:   callsign,
		Module:     module,
		Designator: designator,
		ctx:        ctx,
		cancel:     cancel,
		Packets:    packets,
		Events:     events,
	}
}

func (c *ReflectorClient) Conn() *net.UDPConn {
	return c.UDPConn
}

func (c *ReflectorClient) Addr() *net.UDPAddr {
	return c.RemoteAddr
}

func (c *ReflectorClient) Name() string {
	return c.RemoteAddr.String()
}

func (c *ReflectorClient) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *ReflectorClient) sendControl(build func() ([]byte, error)) error {
	pkt, err := build()
	if err != nil {
		return err
	}
	_, err = c.UDPConn.WriteToUDP(pkt, c.RemoteAddr)
	return err
}

func (c *ReflectorClient) listen() {
	defer close(c.Packets)

	buf := make([]byte, 512)

	for {
		if c.ctx.Err() != nil {
			return
		}

		deadline := time.Now().Add(5 * time.Second)
		if dl, ok := c.ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
		if err := c.UDPConn.SetReadDeadline(deadline); err != nil {
			log.Error("Failed to set read deadline", "err", err, "reflector", c.Designator)
			return
		}

		n, addr, err := c.UDPConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Error("UDP read error", "err", err, "reflector", c.Designator)
			continue
		}

		if c.ctx.Err() != nil {
			return
		}

		if addr.String() != c.RemoteAddr.String() {
			log.Warn("Ignoring packet from unexpected source", "source", addr.String(), "reflector", c.Designator)
			continue
		}

		data := append([]byte(nil), buf[:n]...)

		if n >= 4 && string(data[:4]) == "M17 " {
			select {
			case c.Packets <- data:
			default:
				log.Warn("Packet channel full, dropping stream packet", "reflector", c.Designator)
			}
		} else {
			c.handleControlPacket(data)
		}
	}
}

func (c *ReflectorClient) handleControlPacket(data []byte) {
	ctrlType, callsign, _, err := m17.ParseControlPacket(data)
	if err != nil {
		log.Warn("Unknown/invalid control packet", "err", err, "reflector", c.Designator)
		return
	}

	switch ctrlType {
	case m17.CtrlACKN:
		c.connected = true
		c.lastPing = time.Now()
		log.Info("Reflector ACKN: connected", "callsign", c.Callsign, "reflector", c.Designator)

	case m17.CtrlNACK:
		log.Error("Reflector NACK: connection denied", "reflector", c.Designator)
		select {
		case c.Events <- EventNACK:
		default:
		}
		c.Close()

	case m17.CtrlPING:
		log.Debug("Reflector PING -> sending PONG", "from", callsign, "reflector", c.Designator)
		c.lastPing = time.Now()
		if err := c.sendControl(func() ([]byte, error) {
			return m17.BuildPONG(c.Callsign)
		}); err != nil {
			log.Warn("Failed to send PONG", "err", err, "reflector", c.Designator)
		}

	case m17.CtrlDISC:
		log.Info("Reflector DISC: disconnected by reflector", "reflector", c.Designator)
		c.Close()

	default:
		log.Warn("Unhandled control packet type", "type", ctrlType, "reflector", c.Designator)
	}
}

func (c *ReflectorClient) monitorPing() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if time.Since(c.lastPing) > 30*time.Second && c.connected {
				log.Warn("No PING from reflector; assuming disconnected", "reflector", c.Designator)
				c.Close()
				return
			}
		}
	}
}

func (c *ReflectorClient) Disconnect() {
	c.Close()
}

func (c *ReflectorClient) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		if err := c.sendControl(func() ([]byte, error) {
			return m17.BuildDISC(c.Callsign)
		}); err != nil {
			log.Warn("Error sending DISC", "err", err, "reflector", c.Designator)
		}
		c.UDPConn.Close()
		close(c.Events)
	})
}
