package paac

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync/atomic"

	"github.com/casbin/casbin/v2"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"
)

// A [PAACEndPoint] represents a path aware access control endpoint.
// The endpoint listens for requests at a given SCION address,
// evaluates any incoming requests taking into account
// a set of subject, object and network attributes
// and replies with the result of the access control decision
type PAACEndPoint struct {
	Enforcer       *casbin.SyncedEnforcer
	Endpoint       Endpoint
	NetAttHandler  NetworkAttributeHandler
	SubAttHandler  *GenericAttributeHandler
	ObjAttHandler  *GenericAttributeHandler
	enforcersCount atomic.Int64
	requestCount   atomic.Int64
	Logger         *log.Logger
}

// Can be used to create a new [PAACEndPoint] instead of directly
// interacting with the struct
func NewPAACEndPoint(
	logger *log.Logger,
	enforcer *casbin.SyncedEnforcer,
	Endpoint Endpoint,
	netAttHandler NetworkAttributeHandler,
	subAttHandler *GenericAttributeHandler,
	objAttHandler *GenericAttributeHandler,
) *PAACEndPoint {
	return &PAACEndPoint{
		Enforcer:      enforcer,
		Endpoint:      Endpoint,
		NetAttHandler: netAttHandler,
		SubAttHandler: subAttHandler,
		ObjAttHandler: objAttHandler,
		Logger:        logger,
	}
}

// Starts listening for incoming enforcement requests.
// Starts a number of goroutines for each of the
// component services.
//
// NumEnforcers is the number of goroutines that will be spawned for the casbin enforcer,
// numRequestBuilders is the amount of goroutines that will be processing
// and gathering attributes for incoming requests.
//
// Depending on the use case, different values may be appropriate:
// If the [NetAttHandler] has 100% cache hit rate for paths,
// Setting both to the number of cores on the host should lead to reasonable
// results.
//
// Going from there, as the number of cache misses increases, or if caching is disabled
// entirely, higher values will need to be used as the request request builders
// will be blocking, waiting to receive paths from the SCION daemon.
func (e *PAACEndPoint) Start(numEnforcers, numRequestBuilders int) {
	if numEnforcers <= 0 {
		err := fmt.Errorf("Invalid number of enforcers: %v. Need >=1", numEnforcers)
		if err != nil {
			log.Fatalln("Error creating paac endpoint"+": ", err)
		}
	}
	readPacketChan := e.Endpoint.StartListen()

	enfRequestChan := make(chan *EnforcerRequest, 1)

	// routine to request all relevant attributes and build
	// a request to be forwarded to a casbin enforcer
	requestRoutine := func() {
		pathRequestChan := make(chan *PathRequest, 1)
		pathAttributesChan := e.NetAttHandler.Start(pathRequestChan)
		subRequestChan := make(chan string, 1)
		subAttributesChan := e.SubAttHandler.Start(subRequestChan)
		objRequestChan := make(chan string, 1)
		objAttributesChan := e.ObjAttHandler.Start(objRequestChan)
		if LogLevel >= 1 {
			e.Logger.Println("PAAC Endpoint: started request routine")
		}
		for {
			// get pkt from endpoint
			rPkt, ok := <-readPacketChan
			if !ok {
				if LogLevel >= 1 {
					e.Logger.Println("PAAC Endpoint: readPacketChan closed, closed all outgoing channels and returning")
				}
				close(pathRequestChan)
				close(subRequestChan)
				close(objRequestChan)
				if e.requestCount.Add(-1) == 0 {
					if LogLevel >= 1 {
						e.Logger.Println("PAAC Endpoint: All requestRoutines stopped, closing request channel...")
					}
					close(enfRequestChan)
				}
				return
			}
			// request path attributes
			pathRequestChan <- &PathRequest{
				DataplanePath: rPkt.Pkt.Path,
				PktSource:     rPkt.Pkt.Source.IA,
				PktDest:       rPkt.Pkt.Destination.IA,
			}

			p, ok := rPkt.Pkt.Payload.(snet.UDPPayload)
			if !ok {
				log.Fatalf("Error extracting payload from request packet. Cast failed from %T to snet.UDPPayload", rPkt.Pkt.Payload)
			}

			request, err := DecodeRequest(p.Payload)
			if err != nil {
				log.Fatalln("Failed to extract request from payload:", err)
			}
			if LogLevel >= 5 {
				e.Logger.Printf("Decoded request from packet payload: %+v", request)
			}

			// request other attributes
			subRequestChan <- request.ClientID
			objRequestChan <- request.ObjectID

			er := &EnforcerRequest{
				Request:    request,
				ReadPacket: rPkt,
			}

			subAttrs, ok := <-subAttributesChan
			if !ok {
				log.Fatalln("BUG: Attempted to request subject attributes from closed channel")
			}
			if !subAttrs.Ok {
				log.Fatalf("Error getting subject attributes. Unknown subject: %+v", request.ClientID)
			}
			er.SubAttributes = subAttrs.Attributes

			objAttrs, ok := <-objAttributesChan
			if !ok {
				log.Fatalln("BUG: Attempted to request object attributes from closed channel")
			}
			if !objAttrs.Ok {
				log.Fatalf("Error getting object attributes. Unknown object: %+v", request.ObjectID)
			}
			er.ObjAttributes = objAttrs.Attributes

			pathAttrs, ok := <-pathAttributesChan
			if !ok {
				log.Fatalln("BUG: Attempted to request path attributes from closed channel")
			}
			if pathAttrs.Err != nil {
				log.Fatalln("Error getting path attributes"+": ", pathAttrs.Err)
			}
			er.NetAttributes = pathAttrs.Attributes
			rPkt.ReplyPath = pathAttrs.ReplyPath

			if LogLevel >= 5 {
				e.Logger.Printf("Built enforcer request: \n%+v\n", er)
			}
			enfRequestChan <- er
		}
	}

	e.requestCount.Store(int64(numRequestBuilders))
	for i := 0; i < numRequestBuilders; i++ {
		go requestRoutine()
	}

	e.enforcersCount.Store(int64(numEnforcers))
	enfReplyChan := make(chan *EnforcerReply, 1)
	for i := 0; i < numEnforcers; i++ {
		go e.enforceRoutine(enfRequestChan, enfReplyChan)
	}

	go e.replyRoutine(enfReplyChan)
}

// goroutine interacting with the casbin enforcer to
// evaluate the request
func (e *PAACEndPoint) enforceRoutine(enfRequestChan chan *EnforcerRequest, enfReplyChan chan *EnforcerReply) {
	if LogLevel >= 1 {
		e.Logger.Println("PAAC Endpoint: started enforcer routine")
	}
	for {
		req, ok := <-enfRequestChan
		if !ok {
			if LogLevel >= 1 {
				e.Logger.Println("PAAC Endpoint: request channel closed, enforceRoutine exiting...")
			}
			if e.enforcersCount.Add(-1) == 0 {
				if LogLevel >= 1 {
					e.Logger.Println("PAAC Endpoint: All enforceRoutines stopped, closing reply channel...")
				}
				close(enfReplyChan)
			}
			return
		}

		ok, err := e.Enforcer.Enforce(req.SubAttributes,
			req.ObjAttributes,
			req.NetAttributes,
			req.Request.AccessType,
		)
		er := &EnforcerReply{
			Request: req,
			Reply: &PAACReply{
				Request: req.Request,
				Ok:      ok,
				Err:     err,
			},
		}
		if LogLevel >= 5 {
			e.Logger.Printf("Received enforcer reply: \n%+v\n", er)
		}
		enfReplyChan <- er
	}
}

// goroutine which reads enforcement results from a channel
// and sends a reply packet to the SCION address which made
// the request
func (e *PAACEndPoint) replyRoutine(enfReplyChan chan *EnforcerReply) {
	if LogLevel >= 1 {
		e.Logger.Println("PAAC Endpoint: started reply routine")
	}

	for {
		rep, ok := <-enfReplyChan
		if !ok {
			if LogLevel >= 1 {
				e.Logger.Println("PAAC Endpoint: enfReplyChan closed, returning")
			}
			e.Endpoint.Close()
			return
		}
		rp := rep.Request.ReadPacket
		pkt := rp.Pkt
		ov := rp.Ov
		udp, ok := pkt.Payload.(snet.UDPPayload)
		if !ok {
			log.Fatalf("Error extracting payload from request packet. Cast failed from %T to snet.UDPPayload", pkt.Payload)
		}

		pkt.Destination, pkt.Source = pkt.Source, pkt.Destination

		reply, err := EncodeReply(rep.Reply)
		if err != nil {
			log.Fatalln("Error encoding enforcer reply:", err)
		}
		pkt.Payload = &snet.UDPPayload{
			DstPort: udp.SrcPort,
			SrcPort: udp.DstPort,
			Payload: reply,
		}

		if rp.ReplyPath != nil {
			pkt.Path = rp.ReplyPath
		} else {
			pkt.Path = e.Endpoint.GetReplyPath(rp.Pkt.Path)
		}
		// the path is already reversed when quering for network attributes
		if LogLevel >= 5 {
			e.Logger.Printf("Encoded reply into packet:\n%+v\nsent to: \n%+v\n", pkt, ov)
		}
		err = e.Endpoint.WriteTo(pkt, rp.Ov)
		if err != nil {
			log.Fatalln("Error sending reply packet"+": ", err)
		}
	}
}

// close underlying endpoint and all assosciated PAAC services
func (e *PAACEndPoint) Close() {
	// should result in all other services stopping as
	// closure propagates across channels
	if LogLevel >= 1 {
		e.Logger.Println("PAAC Endpoint: closed")
	}
	e.Endpoint.Close()
}

// struct used to represent an access control request
// currently, attributes can only be retrieved by the endpoint
// when evaluating the request. Sending attributes with the request packet
// is not supported.
type PAACRequest struct {
	ClientID   string
	ObjectID   string
	AccessType string
}

// struct used to represent an access control reply
type PAACReply struct {
	Request *PAACRequest
	Ok      bool
	Err     error
}

// Used to send enforcement requests across a channel
type EnforcerRequest struct {
	Request       *PAACRequest
	NetAttributes map[string]any
	SubAttributes map[string]any
	ObjAttributes map[string]any
	ReadPacket    *ReadPacket
}

// Used to send the result of an enforcement request across a channel
type EnforcerReply struct {
	Request *EnforcerRequest
	Reply   *PAACReply
}

// To be used by a client application to perform access
// control requests
type PAACClient struct {
	Endpoint Endpoint
	Logger   *log.Logger
}

func NewPAACClient(logger *log.Logger, Endpoint Endpoint) *PAACClient {
	return &PAACClient{
		Endpoint: Endpoint,
		Logger:   logger,
	}
}

// Send a single access control request to the given address
// and wait for a reply
func (c *PAACClient) RequestAccess(ctx context.Context, dstIA addr.IA, dstAddress *net.UDPAddr, request *PAACRequest) *PAACReply {
	// encode and send the request struct
	encReq, err := EncodeRequest(request)
	if err != nil {
		log.Fatalln("Error encoding access request"+": ", err)
	}

	c.Endpoint.SendPacket(context.Background(), dstIA, dstAddress, encReq)
	if LogLevel >= 5 {
		c.Logger.Printf("PAAC Client sent request:\n %+v\n", request)
	}

	ov := &net.UDPAddr{}
	pkt := &snet.Packet{}
	// this blocks until a packet is received. Assumes
	// no other packets are sent to this address
	err = c.Endpoint.ReadFrom(pkt, ov)
	if err != nil {
		log.Fatalln("Error reading reply packet from endpoint"+": ", err)
	}

	if LogLevel >= 5 {
		c.Logger.Printf("PAAC Client received reply packet:\n %+v\nfrom: %+v\n", pkt, ov)
	}

	p, ok := pkt.Payload.(snet.UDPPayload)
	if !ok {
		log.Fatalln("Error extracting payload from reply packet")
	}

	reply, err := DecodeReply(p.Payload)
	if err != nil {
		log.Fatalln("Error decoding result of access request"+": ", err)
	}

	if LogLevel >= 5 {
		c.Logger.Printf("PAAC Client received reply:\n %+v\n", reply)
	}
	return reply
}

// Closes the underlying [Endpoint]
func (c *PAACClient) Close() {
	if LogLevel >= 1 {
		c.Logger.Println("PAAC Client: closed")
	}
	c.Endpoint.Close()
}
