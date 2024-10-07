package paac

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync/atomic"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/metrics"
	"github.com/scionproto/scion/pkg/sock/reliable"
)

var (
	scionPacketConnMetrics = metrics.NewSCIONPacketConnMetrics()
	scmpErrorsCounter      = scionPacketConnMetrics.SCMPErrors
)

// See [SCIONEndpoint] for documentation
type Endpoint interface {
	StartListen() chan *ReadPacket
	SendPacket(ctx context.Context, dstIA addr.IA, dstAddress *net.UDPAddr, msgBytes []byte)
	GetReplyPath(path snet.DataplanePath) snet.DataplanePath
	Close()
	WriteTo(pkt *snet.Packet, ov *net.UDPAddr) error
	ReadFrom(pkt *snet.Packet, ov *net.UDPAddr) error
}

// A [SCIONEndpoint] can be used to create a SCION endpoint which serves
// as the interface point with the SCION network, from which packets
// can be sent and received
type SCIONEndpoint struct {
	ctx               context.Context
	LocalAddress      *net.UDPAddr
	LocalIA           addr.IA
	ScionAddress      snet.SCIONAddress
	DaemonConnector   daemon.Connector
	DispatcherService *snet.DefaultPacketDispatcherService
	Connection        snet.PacketConn
	ReplyPather       snet.ReplyPather
	closed            atomic.Bool
	Logger            *log.Logger
}

// Used to send received packets across channels
type ReadPacket struct {
	Ov        *net.UDPAddr
	Pkt       *snet.Packet
	ReplyPath snet.DataplanePath
}

// Returns a new SCION endpoint connected at the given local address
func NewScionEndpoint(ctx context.Context, logger *log.Logger, daemonAddress string, localIA addr.IA, localAddress *net.UDPAddr) *SCIONEndpoint {
	dispatcher := reliable.NewDispatcher("")
	daemonConnector, err := daemon.NewService(daemonAddress).Connect(ctx)
	if err != nil {
		log.Fatalln("Error creating daemon connection factory"+": ", err)
	}

	dispatcherService := &snet.DefaultPacketDispatcherService{
		Dispatcher: dispatcher,
		SCMPHandler: snet.DefaultSCMPHandler{
			RevocationHandler: daemon.RevHandler{Connector: daemonConnector},
			SCMPErrors:        scmpErrorsCounter,
		},
		SCIONPacketConnMetrics: scionPacketConnMetrics,
	}
	connection, port, err := dispatcherService.Register(ctx, localIA, localAddress, addr.SvcNone)
	if err != nil {
		log.Fatalln("Error registering the connection"+": ", err)
	}

	if port != uint16(localAddress.Port) {
		log.Fatalln("Error creating endpoint connection: Endpoint address ports do not match")
	}
	localIP, ok := netip.AddrFromSlice(localAddress.IP)
	if !ok {
		log.Fatalln("Failed to get IP from local address:", localAddress.IP)
	}

	return &SCIONEndpoint{
		ctx:               ctx,
		LocalAddress:      localAddress,
		LocalIA:           localIA,
		ScionAddress:      snet.SCIONAddress{IA: localIA, Host: addr.HostIP(localIP)},
		DispatcherService: dispatcherService,
		Connection:        connection,
		DaemonConnector:   daemonConnector,
		ReplyPather:       snet.DefaultReplyPather{},
		Logger:            logger,
	}
}

// Closes any open connections or channels
func (e *SCIONEndpoint) Close() {
	if !e.closed.CompareAndSwap(false, true) {
		return
	}
	if LogLevel >= 1 {
		e.Logger.Println("SCION Endpoint: closed")
	}
	e.Connection.Close()
	e.DaemonConnector.Close()
}

// Starts a goroutine in the background that will
// read incoming packets from the network, preprocess them and then
// pass them on to the returned channel
func (e *SCIONEndpoint) StartListen() chan *ReadPacket {
	cRead := make(chan *ReadPacket, 1)
	go e.readPacketsRoutine(cRead)
	return cRead
}

// Goroutine to read SCION packets from the network
func (e *SCIONEndpoint) readPacketsRoutine(c chan *ReadPacket) {
	if LogLevel >= 1 {
		e.Logger.Println("SCION Endpoint: started readPackets routine")
	}
	for {
		ov := &net.UDPAddr{}
		pkt := &snet.Packet{}
		err := e.Connection.ReadFrom(pkt, ov)
		if e.closed.Load() {
			if LogLevel >= 1 {
				e.Logger.Println("SCION Endpoint listening connection closed. Closed c and returning")
			}
			close(c)
			return
		}
		if err != nil {
			log.Fatalln("Error reading packet from connection"+": ", err)
		}
		if LogLevel >= 5 {
			e.Logger.Printf("SCION Endpoint read packet: \n%+v\n, \nFrom: \n%+v\n", pkt, ov)
		}
		c <- &ReadPacket{Ov: ov, Pkt: pkt}
	}
}

// This function can be used to send packets over the SCION network
func (e *SCIONEndpoint) SendPacket(ctx context.Context, dstIA addr.IA, dstAddress *net.UDPAddr, msgBytes []byte) {
	paths, err := e.DaemonConnector.Paths(ctx, dstIA, e.LocalIA, daemon.PathReqFlags{})
	if err != nil {
		log.Fatalln("Error getting paths from daemon"+": ", err)
	}
	if len(paths) == 0 {
		err = fmt.Errorf("No known paths from %v to %v", e.LocalIA, dstIA)
	}
	if err != nil {
		log.Fatalln("Error getting paths from daemon"+": ", err)
	}
	dstIP, ok := netip.AddrFromSlice(dstAddress.IP)
	if !ok {
		log.Fatalln("Failed to get IP from local address:", dstAddress.IP)
	}

	path := paths[0]
	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Destination: snet.SCIONAddress{
				IA:   dstIA,
				Host: addr.HostIP(dstIP),
			},
			Source: e.ScionAddress,
			Path:   path.Dataplane(),
			Payload: snet.UDPPayload{
				SrcPort: e.LocalAddress.AddrPort().Port(),
				DstPort: dstAddress.AddrPort().Port(),
				Payload: msgBytes,
			},
		},
	}
	if LogLevel >= 5 {
		e.Logger.Printf("SCION Endpoint sent pkt: \n%+v\nTo:\n%+v\n", pkt, path.UnderlayNextHop())
	}
	err = e.Connection.WriteTo(pkt, path.UnderlayNextHop())
	if err != nil {
		log.Fatalln("Error sending Packet"+": ", err)
	}
}

// Given a DataplanePath, reverses it to get a reply path
func (e *SCIONEndpoint) GetReplyPath(path snet.DataplanePath) snet.DataplanePath {
	rawPath, ok := path.(snet.RawPath)
	if !ok {
		log.Fatalf("Error getting raw path. Failed cast from %T to snet.RawPath", path)
	}
	replyPath, err := e.ReplyPather.ReplyPath(rawPath)
	if err != nil {
		log.Fatalln("Error creating reply path"+": ", err)
	}
	return replyPath
}

// Sends a packet directly to the underlying [PacketConn]
func (e *SCIONEndpoint) WriteTo(pkt *snet.Packet, ov *net.UDPAddr) error {
	return e.Connection.WriteTo(pkt, ov)
}

// Reads a packet directly from the underlying [PacketConn]
func (e *SCIONEndpoint) ReadFrom(pkt *snet.Packet, ov *net.UDPAddr) error {
	return e.Connection.ReadFrom(pkt, ov)
}
