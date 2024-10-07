package paac

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/slayers/path/scion"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"
)

type BenchNetAttributeHandler struct {
	replyPath   snet.DataplanePath
	replyPather snet.ReplyPather
}

// Dummy handler to be used specifically for benchmarking to measure "path-aware" overhead
func NewBenchNetAttributeHandler(replyPath snet.DataplanePath) *BenchNetAttributeHandler {
	return &BenchNetAttributeHandler{
		replyPath:   replyPath,
		replyPather: snet.DefaultReplyPather{},
	}
}

func (n *BenchNetAttributeHandler) UseCache(bool) {}

func (n *BenchNetAttributeHandler) SetCacheExpiry(time.Duration) {}

func (n *BenchNetAttributeHandler) Start(cIn chan *PathRequest) chan *PathAttributes {
	cOut := make(chan *PathAttributes, 1)
	pathAttrRoutine := func() {
		for {
			PathRequest, ok := <-cIn
			if !ok {
				close(cOut)
				return
			}
			cOut <- n.GetPathAttributes(PathRequest)
		}
	}
	go pathAttrRoutine()
	return cOut
}

func (n *BenchNetAttributeHandler) Close() {}

func (n *BenchNetAttributeHandler) GetPathAttributes(p *PathRequest) *PathAttributes {
	m := make(map[string]any)
	rawPath, ok := p.DataplanePath.(snet.RawPath)
	if !ok {
		log.Fatalf("Error getting raw path. Failed cast from %T to snet.RawPath", p.DataplanePath)
	}
	_, err := n.replyPather.ReplyPath(rawPath)
	if err != nil {
		log.Fatalln("Error creating reply path"+": ", err)
	}
	return &PathAttributes{
		Attributes: m,
		ReplyPath:  n.replyPath,
	}
}

func (b *BenchNetAttributeHandler) GetPath(
	dataplanePath snet.DataplanePath,
	pktSource addr.IA,
	pktDest addr.IA,
) (snet.DataplanePath, *scion.Decoded, snet.Path, error) {
	return nil, nil, nil, nil
}

// Dummy endpoint to be used specifically for benchmarking to avoid network overhead
type benchmarkEndpoint struct {
	cIn           chan *ReadPacket
	cOut          chan *ReadPacket
	scionEndpoint *SCIONEndpoint
	closed        atomic.Bool
}

func newBenchmarkEndpoint(cIn chan *ReadPacket, cOut chan *ReadPacket, scionEndpoint *SCIONEndpoint) *benchmarkEndpoint {
	return &benchmarkEndpoint{
		cIn:           cIn,
		cOut:          cOut,
		scionEndpoint: scionEndpoint,
	}
}

func (b *benchmarkEndpoint) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.cOut)
}

func (b *benchmarkEndpoint) StartListen() chan *ReadPacket {
	return b.cIn
}

func (b *benchmarkEndpoint) SendPacket(ctx context.Context, dstIA addr.IA, dstAddress *net.UDPAddr, msgBytes []byte) {
	log.Fatalf(`SendPacket() called on BenchmarkEndpoint. 
    This endpoint is not intended to be used with a PAAC client. 
    Instead, directly send packets to the PAAC server endpoint channel`)
}

func (b *benchmarkEndpoint) GetReplyPath(path snet.DataplanePath) snet.DataplanePath {
	return b.scionEndpoint.GetReplyPath(path)
}

func (b *benchmarkEndpoint) WriteTo(pkt *snet.Packet, ov *net.UDPAddr) error {
	b.cOut <- &ReadPacket{Pkt: pkt, Ov: ov}
	return nil
}

func (b *benchmarkEndpoint) ReadFrom(pkt *snet.Packet, ov *net.UDPAddr) error {
	log.Fatalf(`ReadFrom() called on BenchmarkEndpoint. 
    This endpoint is not intended to be used with a PAAC client. 
    Instead, directly send packets to the PAAC server endpoint channel`)
	return nil
}

func buildRequestReadPacket(scionEndpoint *SCIONEndpoint, dstIA addr.IA, dstAddress *net.UDPAddr, request *PAACRequest) *ReadPacket {
	paths, err := scionEndpoint.DaemonConnector.Paths(context.Background(),
		dstIA,
		scionEndpoint.LocalIA,
		daemon.PathReqFlags{},
	)
	if err != nil {
		log.Fatalln("Error getting paths from daemon"+": ", err)
	}
	if len(paths) == 0 {
		err = fmt.Errorf("No path returned by daemon from src: %v to dst: %v", scionEndpoint.LocalIA, dstIA)
		log.Fatalln("Error getting paths from daemon"+": ", err)
	}

	dstIP, ok := netip.AddrFromSlice(dstAddress.IP)
	if !ok {
		log.Fatalln("Failed to get IP from local address: ", dstAddress.IP)
	}
	payload, err := EncodeRequest(request)
	if err != nil {
		log.Fatalln("Error encoding access request:", err)
	}
	p := paths[0]
	scionPath := p.Dataplane().(path.SCION)
	rawPath := snet.RawPath{PathType: 1, Raw: scionPath.Raw}
	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Destination: snet.SCIONAddress{
				IA:   dstIA,
				Host: addr.HostIP(dstIP),
			},
			Source: scionEndpoint.ScionAddress,
			Path:   rawPath,
			Payload: snet.UDPPayload{
				SrcPort: scionEndpoint.LocalAddress.AddrPort().Port(),
				DstPort: dstAddress.AddrPort().Port(),
				Payload: payload,
			},
		},
	}

	return &ReadPacket{Pkt: pkt, Ov: p.UnderlayNextHop()}
}

func buildRequestReadPacketNoPath(pathSource *ReadPacket, request *PAACRequest) *ReadPacket {
	pki := pathSource.Pkt.PacketInfo
	rawPath := pathSource.Pkt.Path.(snet.RawPath)
	newPathBytes := make([]byte, len(rawPath.Raw))
	copy(newPathBytes, rawPath.Raw)
	newPath := snet.RawPath{PathType: 1, Raw: newPathBytes}
	pl, ok := pki.Payload.(snet.UDPPayload)

	if !ok {
		log.Fatalln("Failed to cast payload when building request packet with no path")
	}

	payload, err := EncodeRequest(request)
	if err != nil {
		log.Fatalln("Error encoding access request:", err)
	}
	pkt := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Destination: pki.Destination,
			Source:      pki.Source,
			Path:        newPath,
			Payload: snet.UDPPayload{
				SrcPort: pl.SrcPort,
				DstPort: pl.DstPort,
				Payload: payload,
			},
		},
	}
	return &ReadPacket{Pkt: pkt, Ov: pathSource.Ov}
}

func createReadPacketWithCopiedPath(rp *ReadPacket) *ReadPacket {
	pki := rp.Pkt.PacketInfo
	rawPath := rp.Pkt.Path.(snet.RawPath)
	newPathBytes := make([]byte, len(rawPath.Raw))
	copy(newPathBytes, rawPath.Raw)
	newPath := snet.RawPath{PathType: 1, Raw: newPathBytes}
	newPacket := &snet.Packet{
		PacketInfo: snet.PacketInfo{
			Destination: pki.Destination,
			Source:      pki.Source,
			Path:        newPath,
			Payload:     pki.Payload,
		},
	}
	return &ReadPacket{Pkt: newPacket, Ov: rp.Ov}
}
