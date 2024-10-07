package paac

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/slayers/path/scion"
	"github.com/scionproto/scion/pkg/snet"
	snpath "github.com/scionproto/scion/pkg/snet/path"
)

type NetworkAttributeHandler interface {
	UseCache(b bool)
	SetCacheExpiry(d time.Duration)
	Start(cIn chan *PathRequest) chan *PathAttributes
	Close()
	GetPathAttributes(pathRequest *PathRequest) *PathAttributes
	GetPath(snet.DataplanePath, addr.IA, addr.IA,
	) (snet.DataplanePath, *scion.Decoded, snet.Path, error)
}

// [SCIONNetworkAttributeHandler] can be used to gather path information
// for incoming SCION packets using the local scion daemon.
// Additional attributes can be manually specified for each path using
// the 'ExternalAttributeHandler'.
//
// This information is packaged as a map of network attributes
// in a [PathAttributes].
// [SCIONNetworkAttributeHandler] *should* be thread-safe
type SCIONNetworkAttributeHandler struct {
	DaemonConnector daemon.Connector
	ReplyPather     snet.ReplyPather
	closed          chan bool
	routineCount    atomic.Int64
	Logger          *log.Logger

	// This [GenericAttributeHandler] can be used by an application to manually
	// set attributes for specific paths. These attributes will be included in
	// any attribute maps returned for those specific paths. If an attribute is
	// both extracted from the metadata and manually specified, the manually set
	// value is returned. It is the responsibility of the application to manage
	// these attributes.
	ExternalAttributeHandler *GenericAttributeHandler
	pathCache                map[string]*pathCacheEntry
	tmpOldCache              map[string]*pathCacheEntry
	cacheExpiry              time.Duration
	cacheLock                sync.RWMutex
	cleanupLock              sync.Mutex
	cleanupQuit              chan bool
}

// Contains path attributes and any error encountered while collecting
// those attributes.
type PathAttributes struct {
	ReplyPath snet.DataplanePath
	// A set of network attributes
	// See comments in [NetworkAttributeHandler.GetPathAttributes]
	// for a description of each attribute
	Attributes map[string]any
	Err        error
}

// Used to transport relevant query information across a channel
type PathRequest struct {
	DataplanePath snet.DataplanePath
	PktSource     addr.IA
	PktDest       addr.IA
}

// Creates a new [SCIONNetworkAttributeHandler] quering the given daemon for
// path information.
func NewNetworkAttributeHandler(ctx context.Context, logger *log.Logger, daemonAddress string) *SCIONNetworkAttributeHandler {
	daemonConnector, err := daemon.NewService(daemonAddress).Connect(ctx)
	if err != nil {
		log.Fatalln(fmt.Sprintf("Error creating daemon connection factory with daemon address %v\n", daemonAddress)+": ", err)
	}

	return &SCIONNetworkAttributeHandler{
		DaemonConnector: daemonConnector,
		ReplyPather:     snet.DefaultReplyPather{},
		Logger:          logger,
		closed:          make(chan bool),
	}
}

type pathCacheEntry struct {
	path   snet.Path
	expiry time.Time
}

// Set whether path attributes should be cached or fetched from the SCION daemon on every request
// By default, paths are not cached.
// If caching is enabled, paths are cached until the expiry time set in their metadata by default,
// capped at 24h
// The cache expiry can be changed with [SetCacheExpiry], but will never exceed
// the expiry time set in the path metadata for any given entry.
func (n *SCIONNetworkAttributeHandler) UseCache(b bool) {
	n.cleanupLock.Lock()
	if b {
		n.cacheLock.Lock()
		if n.pathCache == nil {
			if n.cacheExpiry == 0 {
				n.cacheExpiry = 24 * time.Hour
			}
			if n.cleanupQuit != nil {
				close(n.cleanupQuit)
			}
			n.cleanupQuit = make(chan bool)
			go n.cacheCleanupRoutine(n.cacheExpiry, n.cleanupQuit)
			n.pathCache = make(map[string]*pathCacheEntry)
			if LogLevel >= 2 {
				n.Logger.Println("Network attribute handler: caching enabled")
			}
		}
		n.cacheLock.Unlock()
		n.cleanupLock.Unlock()
		return
	}
	n.cacheLock.Lock()
	if n.pathCache != nil {
		if LogLevel >= 2 {
			n.Logger.Println("Network attribute handler: caching disabled")
		}
		n.pathCache = nil
	}
	n.cacheLock.Unlock()
	n.cleanupLock.Unlock()
}

func (n *SCIONNetworkAttributeHandler) cacheCleanupRoutine(interval time.Duration, c chan bool) {
	t := time.NewTimer(interval)
	if LogLevel >= 2 {
		n.Logger.Printf("Network attribute handler: started cleanup routine, interval=%v\n", interval)
	}
	for {
		select {
		case <-c:
			if LogLevel >= 2 {
				n.Logger.Println("Network attribute handler: killed cleanup routine")
			}
			return
		case <-t.C:
			n.cleanupLock.Lock()
			t.Reset(interval)
			n.cacheLock.Lock()
			n.tmpOldCache = maps.Clone(n.pathCache)
			localCopy := maps.Clone(n.pathCache)
			n.pathCache = make(map[string]*pathCacheEntry)
			n.cacheLock.Unlock()
			if LogLevel >= 2 {
				n.Logger.Printf("Network attribute handler: Starting cleanup. Current cache size: %v\n", len(localCopy))
			}
			tn := time.Now()
			for hash, pce := range localCopy {
				if pce.expiry.Before(tn) {
					delete(localCopy, hash)
				}
			}
			n.cacheLock.Lock()
			diff := len(localCopy) - len(n.tmpOldCache)
			maps.Copy(localCopy, n.pathCache)
			n.pathCache = localCopy
			if LogLevel >= 2 {
				n.Logger.Printf("Network attribute handler: purged %v/%v expired paths from cache\n", diff, len(n.tmpOldCache))
			}
			n.tmpOldCache = nil
			n.cacheLock.Unlock()
			n.cleanupLock.Unlock()
		}
	}
}

// Sets the expiry for the path cache. Any existing entries will have their expiry updated
// if their remaining validity is longer than the provided duration.
// Expiry will never exceed the expiry time set in the path metadata.
func (n *SCIONNetworkAttributeHandler) SetCacheExpiry(d time.Duration) {
	if n.cacheExpiry == d {
		return
	}
	n.cleanupLock.Lock()
	n.cacheLock.Lock()
	n.cacheExpiry = d

	if n.pathCache == nil {
		n.cacheLock.Unlock()
		return
	}

	if n.cleanupQuit != nil {
		close(n.cleanupQuit)
	}
	n.cleanupQuit = make(chan bool)
	go n.cacheCleanupRoutine(n.cacheExpiry, n.cleanupQuit)

	newExpires := time.Now().Add(d)
	for _, v := range n.pathCache {
		v.expiry = minTime(v.expiry, newExpires)
	}
	n.cacheLock.Unlock()
	n.cleanupLock.Unlock()
	if LogLevel >= 2 {
		n.Logger.Printf("Network attribute handler: cache expiry set to %v\n", d)
	}
}

// [SCIONNetworkAttributeHandler.Start] starts a goroutine in the background that will
// read PathRequests from the input channel cIn, get their PathAttributes and write them
// to the output channel cOut
func (n *SCIONNetworkAttributeHandler) Start(cIn chan *PathRequest) chan *PathAttributes {
	cOut := make(chan *PathAttributes, 1)
	n.routineCount.Add(1)
	go n.getPathAttributesRoutine(cIn, cOut)
	return cOut
}

// Close the connection to the SCION daemon and stop any running handlers
func (n *SCIONNetworkAttributeHandler) Close() {
	if n.routineCount.Add(-1) != 0 {
		return
	}
	if LogLevel >= 1 {
		n.Logger.Println("Network attribute handler: closed")
	}
	n.cleanupLock.Lock()
	close(n.cleanupQuit)
	n.cleanupLock.Unlock()

	close(n.closed)
	n.DaemonConnector.Close()
}

// Reads incoming PathRequests, gets their path attributes and writes them to cOut
func (n *SCIONNetworkAttributeHandler) getPathAttributesRoutine(cIn chan *PathRequest, cOut chan *PathAttributes) {
	if LogLevel >= 1 {
		n.Logger.Println("Network attribute handler: started routine")
	}
	for {
		select {
		case pathRequest, ok := <-cIn:
			if !ok {
				if LogLevel >= 1 {
					n.Logger.Println("Network attribute handler: cIn closed, closed cOut and returning")
				}
				close(cOut)
				n.Close()
				return
			}
			cOut <- n.GetPathAttributes(pathRequest)
		case <-n.closed:
			close(cOut)
			n.Close()
			return
		}
	}
}

// Takes a decoded packet, builds a sequence of ingress/egress interfaces out of its hop fields
// and uses this to create a hash uniquely identifying the path
func buildHash(decoded *scion.Decoded) []byte {
	hash := sha256.New()
	var currentIF int8 = 0
	var hopInSegment uint8 = 0
	for _, hopField := range decoded.HopFields {
		if1, if2 := hopField.ConsIngress, hopField.ConsEgress
		if !decoded.InfoFields[currentIF].ConsDir {
			if1, if2 = if2, if1
		}

		err := binary.Write(hash, binary.BigEndian, if1)
		if err != nil {
			log.Fatalln("Failed writing to hash"+": ", err)
		}
		err = binary.Write(hash, binary.BigEndian, if2)
		if err != nil {
			log.Fatalln("Failed writing to hash"+": ", err)
		}

		hopInSegment += 1
		if hopInSegment == decoded.PathMeta.SegLen[currentIF] {
			hopInSegment = 0
			currentIF += 1
		}
	}
	pathHash := hash.Sum(nil)
	return pathHash
}

// Does the same as [buildHash], but reads the path in reversed order in order to match an incoming
// path with an outgoing one. This is to avoid having
// to get a reversed reply path before actually handling the enforcement request.
func buildReverseHash(decoded *scion.Decoded) []byte {
	hash := sha256.New()
	var currentIF int8 = int8(decoded.NumINF) - 1
	var hopInSegment uint8 = decoded.PathMeta.SegLen[currentIF]
	hops := decoded.HopFields
	for i := len(hops) - 1; i >= 0; i-- {
		if1, if2 := hops[i].ConsIngress, hops[i].ConsEgress
		if decoded.InfoFields[currentIF].ConsDir {
			if1, if2 = if2, if1
		}

		err := binary.Write(hash, binary.BigEndian, if1)
		if err != nil {
			log.Fatalln("Failed writing to hash"+": ", err)
		}
		err = binary.Write(hash, binary.BigEndian, if2)
		if err != nil {
			log.Fatalln("Failed writing to hash"+": ", err)
		}

		hopInSegment -= 1
		if hopInSegment == 0 {
			currentIF -= 1
			if currentIF < 0 {
				break
			}
			hopInSegment = decoded.PathMeta.SegLen[currentIF]
		}
	}
	pathHash := hash.Sum(nil)
	return pathHash
}

// Given path information of a received packet, attempts to match its dataplane path with
// a path known to the local SCION daemon.
// Returns an error if no such path is found.
//
// TODO: for efficiency, return the decoded dataplane path so that it can be used to build
// a reply directly without having to re-decode the raw path
func (n *SCIONNetworkAttributeHandler) GetPath(
	dataplanePath snet.DataplanePath,
	pktSource addr.IA,
	pktDest addr.IA,
) (snet.DataplanePath, *scion.Decoded, snet.Path, error) {
	rawPath, ok := dataplanePath.(snet.RawPath)
	if rawPath.PathType != scion.PathType {
		log.Fatalln("Dataplane path is not SCION type. Instead has type", rawPath.PathType)
	}
	if !ok {
		log.Fatalf("Error getting raw path. Failed cast from %T to snet.RawPath", dataplanePath)
	}
	decoded := &scion.Decoded{}
	err := decoded.DecodeFromBytes(rawPath.Raw)
	if err != nil {
		log.Fatalln(fmt.Sprintf("Error decoding dataplane path:\n %+v\n", rawPath)+": ", err)
	}

	pathHash := buildHash(decoded)
	// check for path in cache
	n.cacheLock.RLock()
	if n.pathCache != nil {
		cachedPath, ok := n.pathCache[string(pathHash)]
		if !ok && n.tmpOldCache != nil {
			cachedPath, ok = n.tmpOldCache[string(pathHash)]
		}
		if ok {
			if !time.Now().After(cachedPath.expiry) {
				n.cacheLock.RUnlock()
				if LogLevel >= 5 {
					n.Logger.Printf("Retrieved matching path from cache:\n%+v\n", cachedPath.path)
				}
				return cachedPath.path.Dataplane(), decoded, cachedPath.path, nil
			}
		}
	}
	n.cacheLock.RUnlock()

	// TODO Should Refresh be set in PathReqFlags?
	paths, err := n.DaemonConnector.Paths(context.Background(), pktSource, pktDest, daemon.PathReqFlags{})
	if err != nil {
		log.Fatalln(fmt.Sprintf("Error getting paths from daemon when matching paths for src=%+v, dst=%+v", pktSource, pktDest)+": ", err)
	}

	if len(paths) == 0 {
		return nil, nil, nil, fmt.Errorf("Daemon returned empty path list when matching paths for src=%+v, dst=%+v", pktSource, pktDest)
	}

	var eq bool
	var path snet.Path

	for _, p := range paths {
		rp, ok := p.Dataplane().(snpath.SCION)
		if !ok {
			log.Fatalf("Failed casting received daemon dataplane path of type %T to path.SCION when matching paths", p.Dataplane())
		}

		decodedD := &scion.Decoded{}
		err = decodedD.DecodeFromBytes(rp.Raw)
		if err != nil {
			log.Fatalln("Error decoding received raw dataplane path when matching paths"+": ", err)
		}

		h := buildReverseHash(decodedD)

		if eq = bytes.Equal(pathHash[:], h[:]); eq {
			path = p
			n.cacheLock.Lock()
			if n.pathCache != nil {
				n.pathCache[string(pathHash)] = &pathCacheEntry{path, minTime(path.Metadata().Expiry, time.Now().Add(n.cacheExpiry))}
			}
			n.cacheLock.Unlock()
			break
		}
	}

	if !eq {
		err = fmt.Errorf("No matching path found out of %v returned, for path: %+v\n", len(paths), decoded)
		if LogLevel >= 5 {
			n.Logger.Printf("Daemon: no matching path found for path: \n%+v\n", decoded)
		}
	} else {
		if LogLevel >= 5 {
			n.Logger.Printf("Retrieved matching path from daemon:\n%+v\nOriginal dataplane path:\n%+v\nReturning reply dataplane path:\n%+v\n", path, dataplanePath, path.Dataplane())
		}
	}
	return path.Dataplane(), decoded, path, err
}

// Given path information of a received packet, attempts to match
// it with known paths returned from the SCION daemon. If a matching path is found, metadata extracted
// from the daemon path is combined with any manually set path attributes to create
// an attribute map.
// If no matching path could be found, an error is returned.
func (n *SCIONNetworkAttributeHandler) GetPathAttributes(pathRequest *PathRequest) *PathAttributes {
	replyPath, decoded, daemonPathToSource, err := n.GetPath(pathRequest.DataplanePath, pathRequest.PktSource, pathRequest.PktDest)
	if err != nil {
		return &PathAttributes{replyPath, nil, err}
	}

	m := make(map[string]any)

	// Source IA as string. We do not care about destination IA (the IA where the PAAC
	// system is running).
	// Since the metadata is from the perspective of the local SCION daemon,
	// destination and source need to be reversed.
	// Any other directional metadata would also need to be reversed.
	// This is due to an unfortunate limitation of the current SCION implementation
	// where we can only get metadata about a path from the daemon as
	// the packet itself only includes the basic forwarding path
	m["SrcIA"] = daemonPathToSource.Destination().String()
	mtd := daemonPathToSource.Metadata()

	// MTU in bytes
	m["MTU"] = mtd.MTU

	// Expiry as Unix timestamp (seconds)
	m["Expiry"] = mtd.Expiry.Unix()

	// (minimum) Latency in nanoseconds
	ltcy := 0
	for _, linkLatency := range mtd.Latency {
		if linkLatency != snet.LatencyUnset {
			ltcy += ltcy
		}
	}
	m["Latency"] = ltcy

	// Minimum announced bandwidth in Kbit/s
	m["Bandwidth"] = slices.Min(mtd.Bandwidth)

	// A boolean describing whether the path contains parts that may be routed
	// over the open internet
	var linkType snet.LinkType
	for _, link := range mtd.LinkType {
		linkType = max(link, linkType)
	}
	m["LinkType"] = int(linkType)

	// Number of HopFields in the path
	m["Hops"] = decoded.NumHops

	// Number of AS internal hops on the path, excluding first and last AS
	iHops := 0
	for _, ASHops := range mtd.InternalHops {
		iHops += int(ASHops)
	}
	m["InternalHops"] = iHops

	// List of ASes on the path
	asList := []string{}
	if len(mtd.Interfaces) >= 1 {
		asList = append(asList, mtd.Interfaces[0].String())
		for i := 1; i < len(mtd.Interfaces); i++ {
			intf := mtd.Interfaces[i].String()
			if intf != asList[i-1] {
				asList = append(asList, intf)
			}
		}
	}
	m["AsList"] = asList

	if LogLevel >= 5 {
		n.Logger.Printf("Retrieved network attributes for path=\n%+v \nattributes=\n%v\n", daemonPathToSource, m)
	}

	// If any additional path attributes are manually set by an external
	// application, add those.
	// Manually set attributes overwrite any that are extracted from metadata
	if n.ExternalAttributeHandler != nil {
		attrs := n.ExternalAttributeHandler.Get(string(snet.Fingerprint(daemonPathToSource)))
		if attrs.Ok {
			maps.Copy(m, attrs.Attributes)
		}
	}
	return &PathAttributes{replyPath, m, nil}
}
