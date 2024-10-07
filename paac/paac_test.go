package paac

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/scionproto/scion/pkg/addr"
)

var (
	repeatCount       = 30
	repeatCountSingle = 30
	attributeMapSize  = 40
	rules             = []int{1, 10, 100, 1000}
	attrs             = []int{2, 4, 8, 12, 20}
	falseTrue         = []bool{false, true}
)

var scionDir = flag.String("scionDir", "", "Scionproto location")

func runScionCommand(name string, args ...string) {
	oldWd, err := os.Getwd()
	if err != nil {
		log.Fatalln("Error getting working directory"+": ", err)
	}
	err = os.Chdir(*scionDir)
	if err != nil {
		log.Fatalln("Error changing to scion directory: "+*scionDir+": ", err)
	}

	cmd := exec.Command(name, args...)
	_, err = cmd.Output()
	if err != nil {
		log.Fatalln("Error executing command"+": ", err)
	}

	err = os.Chdir(oldWd)
	if err != nil {
		log.Fatalln("Error changing back to old working directory"+": ", err)
	}
}

func buildPolicyCsv(path string, random *rand.Rand, numRules int, subAttrs, objAttrs map[string]any, netAttrs bool) {
	if _, err := os.Stat(path + ".csv"); err == nil {
		return
	}
	csvFile, err := createDir(path + ".csv")
	if err != nil {
		log.Fatalln(fmt.Sprintf("failed creating file %q: %s", path, err)+": ", err)
	}

	csvwriter := csv.NewWriter(csvFile)
	var key string
	for i := 1; i <= numRules; i++ {
		subS := ""
		for j, m := range []map[string]any{subAttrs, objAttrs} {
			for k := range m {
				if subS != "" {
					subS += " || "
				}
				if j == 0 {
					key = "r.sub." + k
				} else {
					key = "r.obj." + k
				}
				if strings.HasPrefix(k, "String") {
					subS += fmt.Sprintf("%v %v 'SomeLongDefaultString%v' ", key, "==", strconv.Itoa(-i))
				} else if strings.HasPrefix(k, "Int") {
					subS += fmt.Sprintf("%v %v %v", key, "<=", -i)
				} else if strings.HasPrefix(k, "Float") {
					subS += fmt.Sprintf("%v %v %.2f", key, "<=", float64(-i))
				}
			}
		}
		if netAttrs {
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.SrcIA %v '0-ff00:0:00%v'", "==", random.Intn(10))
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.MTU %v %v", "<=", -i)
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.Expiry %v %v", "<=", -i)
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.Latency %v %v", "<=", -(i + 1))
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.Bandwidth %v %v", "<=", -i)
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.LinkType %v %v", "<=", -1)
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.Hops %v %v", "<=", -i)
			subS += " " + "||" + " "
			subS += fmt.Sprintf("r.net.InternalHops %v %v", "<=", -i)
		}

		err := csvwriter.Write([]string{"p", subS, "read", "allow"})
		if err != nil {
			log.Fatalln("Failed writing to csv"+": ", err)
		}
	}
	csvwriter.Flush()
	csvFile.Close()
}

func newReqAttrs(random *rand.Rand, size int) map[string]any {
	return newAttributes(random, max(size, attributeMapSize))
}

func newAttributes(random *rand.Rand, size int) map[string]any {
	attMap := make(map[string]any)

	var key string
	var val any
	for i := 0; i < size; i++ {
		if i%3 == 1 {
			key = "StringField" + strconv.Itoa(i/3)
			val = "SomeLongDefaultString" + strconv.Itoa(random.Intn(max(i/3, 2)))
		} else if i%3 == 0 {
			key = "IntField" + strconv.Itoa(i/3)
			val = random.Intn(max(i/3, 2))
		} else {
			key = "FloatField" + strconv.Itoa(i/3)
			val = random.Float64()
		}
		attMap[key] = val
	}
	return attMap
}

func benchmarkPAAC(policy_i, numRules, numAtts int, b *testing.B, random *rand.Rand, netAttr, useCache, benchAcs, benchLatency bool, numEnforcers, numBuilders int) {
	fname := fmt.Sprintf("bench_files/policies/set%dNet%v/%dRules%dAtts", policy_i, netAttr, numRules, numAtts)

	// Use some addresses from wide.topo
	// serverIA, err := addr.ParseIA("3-ff00:0:312")
	serverIA, err := addr.ParseIA("1-ff00:0:110")
	if err != nil {
		log.Fatalln(err)
	}
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:6969")
	if err != nil {
		log.Fatalln(err)
	}
	serverDaemonAddress := "127.0.0.11:30255"

	// clientIA, err := addr.ParseIA("1-ff00:0:111")
	clientIA, err := addr.ParseIA("1-ff00:0:111")
	if err != nil {
		log.Fatalln(err)
	}
	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:6969")
	if err != nil {
		log.Fatalln(err)
	}
	clientDaemonAddress := "127.0.0.19:30255"
	// Create logger, we will use the same one for all endpoints/services
	logger := log.New(os.Stdout, fmt.Sprintf("INFO-lvl%v: ", LogLevel), log.Ltime|log.Lmicroseconds|log.Lshortfile)

	// Create SCION endpoints from which we can send/receive SCION packets
	se := NewScionEndpoint(context.Background(), logger, serverDaemonAddress, serverIA, serverAddr)
	ce := NewScionEndpoint(context.Background(), logger, clientDaemonAddress, clientIA, clientAddr)
	defer se.Close()
	defer ce.Close()
	cSend := make(chan *ReadPacket, 1)
	cRcv := make(chan *ReadPacket, 1)
	serverEndpoint := newBenchmarkEndpoint(cSend, cRcv, se)
	defer serverEndpoint.Close()

	// Initialize a basic casbin SyncedEnforcer. For more information about casbin model configuration
	// and policy definition, see https://casbin.org/docs/overview.
	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		buildPolicyCsv(fname, random, numRules, newAttributes(random, numAtts/2), newAttributes(random, numAtts/2), netAttr)
	}
	enforcer, err := casbin.NewSyncedEnforcer("bench_files/bench_model.conf", fname+".csv")
	if err != nil {
		log.Fatalln(err)
	}

	baseReadPacket := buildRequestReadPacket(ce, serverIA, serverAddr, &PAACRequest{})

	var netAttHandler NetworkAttributeHandler
	if benchAcs {
		pathPkt := createReadPacketWithCopiedPath(baseReadPacket)
		replyPath := se.GetReplyPath(pathPkt.Pkt.Path)
		netAttHandler = NewBenchNetAttributeHandler(replyPath)
	} else {
		netAttHandler = NewNetworkAttributeHandler(context.Background(), logger, serverDaemonAddress)
	}
	netAttHandler.UseCache(useCache)
	netAttHandler.SetCacheExpiry(1 * time.Hour)
	// This handler will store information for our request subjects
	subAttHandler := NewGenericAttributeHandler(logger, newReqAttrs(random, numAtts/2))
	// This handler will store information for our request objects
	objAttHandler := NewGenericAttributeHandler(logger, newReqAttrs(random, numAtts/2))
	paacEndpoint := NewPAACEndPoint(logger, enforcer, serverEndpoint, netAttHandler, subAttHandler, objAttHandler)
	paacEndpoint.Start(numEnforcers, numBuilders)
	defer paacEndpoint.Close()

	requests := make([]*ReadPacket, b.N)
	for i := range b.N {
		id := strconv.Itoa(i)
		err := subAttHandler.Put(id, newReqAttrs(random, numAtts/2))
		if err != nil {
			log.Fatalln("Failed putting subject attribute into handler"+": ", err)
		}
		err = objAttHandler.Put(id, newReqAttrs(random, numAtts/2))
		if err != nil {
			log.Fatalln("Failed putting object attribute into handler"+": ", err)
		}

		requests[i] = buildRequestReadPacketNoPath(
			baseReadPacket,
			&PAACRequest{
				ClientID:   id,
				ObjectID:   id,
				AccessType: "read",
			})
	}
	tRP := createReadPacketWithCopiedPath(baseReadPacket)
	_, _, _, err = netAttHandler.GetPath(tRP.Pkt.Path, clientIA, serverIA)
	if err != nil {
		log.Fatalln("Failed populating path cache"+": ", err)
	}

	b.ResetTimer()

	if !benchLatency {
		go func() {
			for i := range b.N {
				cSend <- requests[i]
			}
			close(cSend)
		}()
		for range b.N {
			<-cRcv
		}
	} else {
		for i := range b.N {
			cSend <- requests[i]
			<-cRcv
		}
		close(cSend)
	}
}

func BenchmarkPAACNet(b *testing.B) {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalln("Failed to get working directory"+": ", err)
	}

	runScionCommand("./scion.sh", "topology", "-c", filepath.Dir(dir)+"/topo/tiny.topo")
	runScionCommand("./scion.sh", "run")
	time.Sleep(10 * time.Second)
	for _, benchLatency := range falseTrue {
		for _, r := range rules {
			for _, a := range attrs {
				for i := range repeatCount {
					random := rand.New(rand.NewSource(int64(i)))
					name := fmt.Sprintf("BenchmarkPAACNet%dRules%dAttrs", r, a)
					benchFunc := func(barg *testing.B) {
						benchmarkPAAC(i, r, a, barg, random, true, true, false, benchLatency, runtime.NumCPU(), runtime.NumCPU())
					}
					b.Run(name, benchFunc)

				}
			}
		}
	}
}

func BenchmarkPAACNoNetNoCache(b *testing.B) {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalln("Failed to get working directory"+": ", err)
	}
	runScionCommand("./scion.sh", "topology", "-c", filepath.Dir(dir)+"/topo/tiny.topo")
	runScionCommand("./scion.sh", "run")
	time.Sleep(10 * time.Second)

	for _, benchLatency := range falseTrue {
		for parallelism := 1; parallelism <= 5; parallelism += 2 {
			for _, r := range rules {
				for _, a := range attrs {
					for i := range repeatCount {
						random := rand.New(rand.NewSource(int64(i)))
						name := fmt.Sprintf("BenchmarkPAACNoNetNoCache%dRules%dAttrs%dBuilders", r, a, parallelism)
						if benchLatency {
							name += "Latency"
						}
						benchFunc := func(barg *testing.B) {
							benchmarkPAAC(i, r, a, barg, random, false, false, false, benchLatency, runtime.NumCPU(), parallelism*runtime.NumCPU())
						}
						b.Run(name, benchFunc)
					}
				}
			}
		}
	}
}

func BenchmarkPAACNoNet(b *testing.B) {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalln("Failed to get working directory"+": ", err)
	}

	runScionCommand("./scion.sh", "topology", "-c", filepath.Dir(dir)+"/topo/tiny.topo")
	runScionCommand("./scion.sh", "run")
	time.Sleep(10 * time.Second)
	for _, benchLatency := range falseTrue {
		for _, r := range rules {
			for _, a := range attrs {
				for i := range repeatCount {
					random := rand.New(rand.NewSource(int64(i)))
					name := fmt.Sprintf("BenchmarkPAACNoNet%dRules%dAttrs", r, a)
					numEnf := runtime.NumCPU()
					if benchLatency {
						name += "Latency"
						numEnf = 1
					}
					benchFunc := func(barg *testing.B) {
						benchmarkPAAC(i, r, a, barg, random, false, true, false, benchLatency, numEnf, numEnf)
					}
					b.Run(name, benchFunc)
				}
			}
		}
	}
}

func BenchmarkACS(b *testing.B) {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalln("Failed to get working directory"+": ", err)
	}

	runScionCommand("./scion.sh", "topology", "-c", filepath.Dir(dir)+"/topo/tiny.topo")
	runScionCommand("./scion.sh", "run")
	time.Sleep(10 * time.Second)
	for _, benchLatency := range falseTrue {
		for _, r := range rules {
			for _, a := range attrs {
				for i := range repeatCount {
					random := rand.New(rand.NewSource(int64(i)))
					name := fmt.Sprintf("BenchmarkACS%dRules%dAttrs", r, a)
					numEnf := runtime.NumCPU()
					if benchLatency {
						name += "Latency"
						numEnf = 1
					}
					benchFunc := func(barg *testing.B) {
						benchmarkPAAC(i, r, a, barg, random, false, true, true, benchLatency, numEnf, numEnf)
					}
					b.Run(name, benchFunc)
				}
			}
		}
	}
}

func benchmarkBaseline(policy_i, numRules, numAtts int, b *testing.B, random *rand.Rand) {
	fname := fmt.Sprintf("bench_files/policies/set%dNet%v/%dRules%dAtts", policy_i, false, numRules, numAtts)

	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		buildPolicyCsv(fname, random, numRules, newAttributes(random, numAtts/2), newAttributes(random, numAtts/2), false)
	}
	enforcer, err := casbin.NewEnforcer("bench_files/bench_model.conf", fname+".csv")
	if err != nil {
		log.Fatalln(err)
	}
	subReqs := make([]map[string]any, b.N)
	objReqs := make([]map[string]any, b.N)
	for i := range b.N {
		subReqs[i] = newReqAttrs(random, numAtts/2)
		objReqs[i] = newReqAttrs(random, numAtts/2)
	}
	netAtt := make(map[string]any)

	b.ResetTimer()

	for i := range b.N {
		_, err := enforcer.Enforce(subReqs[i], objReqs[i], netAtt, "read")
		if err != nil {
			log.Fatalln("Enforcer error"+": ", err)
		}
	}
}

func BenchmarkBaseline(b *testing.B) {
	for _, r := range rules {
		for _, a := range attrs {
			for i := range repeatCount {
				random := rand.New(rand.NewSource(int64(i)))
				name := fmt.Sprintf("BenchmarkBaseline%dRules%dAttrs", r, a)
				benchFunc := func(barg *testing.B) {
					benchmarkBaseline(i, r, a, barg, random)
				}
				b.Run(name, benchFunc)
			}
		}
	}
}

func benchmarkBaselineParallel(policy_i, numRules, numAtts int, b *testing.B, random *rand.Rand) {
	fname := fmt.Sprintf("bench_files/policies/set%dNet%v/%dRules%dAtts", policy_i, false, numRules, numAtts)

	if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
		buildPolicyCsv(fname, random, numRules, newAttributes(random, numAtts/2), newAttributes(random, numAtts/2), false)
	}
	enforcer, err := casbin.NewSyncedEnforcer("bench_files/bench_model.conf", fname+".csv")
	if err != nil {
		log.Fatalln(err)
	}
	subReqs := make([]map[string]any, b.N)
	objReqs := make([]map[string]any, b.N)
	for i := range b.N {
		subReqs[i] = newReqAttrs(random, numAtts/2)
		objReqs[i] = newReqAttrs(random, numAtts/2)
	}
	netAtt := make(map[string]any)
	var bi atomic.Uint64
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(bi.Add(1)) - 1
			_, err := enforcer.Enforce(subReqs[i], objReqs[i], netAtt, "read")
			if err != nil {
				log.Fatalln("Enforcer error"+": ", err)
			}
		}
	})
}

func BenchmarkBaselineParallel(b *testing.B) {
	for _, r := range rules {
		for _, a := range attrs {
			for i := range repeatCount {
				random := rand.New(rand.NewSource(int64(i)))
				name := fmt.Sprintf("BenchmarkBaselineParallel%dcores%dRules%dAttrs", runtime.NumCPU(), r, a)
				benchFunc := func(barg *testing.B) {
					benchmarkBaselineParallel(i, r, a, barg, random)
				}
				b.Run(name, benchFunc)

			}
		}
	}
}

func BenchmarkPAACSinglePath(b *testing.B) {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalln("Failed to get working directory"+": ", err)
	}
	runScionCommand("./scion.sh", "topology", "-c", filepath.Dir(dir)+"/topo/single_path_test.topo")

	file, err := os.Open(*scionDir + "/gen/sciond_addresses.json")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	var sciond_addr map[string]any

	// Decode the JSON data into the map
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&sciond_addr)
	if err != nil {
		fmt.Println("Error decoding JSON:", err)
		return
	}

	rules := 1
	att := 1

	for _, benchLatency := range falseTrue {
		for as := 0; as <= 10; as++ {
			runScionCommand("./scion.sh", "run")
			time.Sleep(10 * time.Second)
			random := rand.New(rand.NewSource(int64(as)))

			sAddr := fmt.Sprintf("1-ff00:0:1%02d", as)
			cAddr := fmt.Sprintf("3-ff00:0:3%02d", as)

			numHops := 2 + 2*as
			name := fmt.Sprintf("BenchmarkPAACSinglePath%vHops%vRules%vAttrs", numHops, rules, att)
			if benchLatency {
				name += "Latency"
			}

			serverIA, err := addr.ParseIA(sAddr)
			if err != nil {
				log.Fatalln(err)
			}
			serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:6969")
			if err != nil {
				log.Fatalln(err)
			}
			serverDaemonAddress := sciond_addr[sAddr].(string) + ":30255"

			clientIA, err := addr.ParseIA(cAddr)
			if err != nil {
				log.Fatalln(err)
			}
			clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:6969")
			if err != nil {
				log.Fatalln(err)
			}
			clientDaemonAddress := sciond_addr[cAddr].(string) + ":30255"

			LogLevel = 0

			logger := log.New(os.Stdout, fmt.Sprintf("INFO-lvl%v: ", LogLevel), log.Ltime|log.Lmicroseconds|log.Lshortfile)

			se := NewScionEndpoint(context.Background(), logger, serverDaemonAddress, serverIA, serverAddr)
			ce := NewScionEndpoint(context.Background(), logger, clientDaemonAddress, clientIA, clientAddr)
			defer se.Close()
			defer ce.Close()

			netAttHandler := NewNetworkAttributeHandler(context.Background(), logger, serverDaemonAddress)
			netAttHandler.UseCache(true)
			netAttHandler.SetCacheExpiry(1 * time.Hour)
			baseReadPacket := buildRequestReadPacket(ce, serverIA, serverAddr, &PAACRequest{})
			// populate cache
			tRP := createReadPacketWithCopiedPath(baseReadPacket)
			_, _, _, err = netAttHandler.GetPath(tRP.Pkt.Path, clientIA, serverIA)
			if err != nil {
				log.Fatalln("Failed populating path cache"+": ", err)
			}

			runScionCommand("./scion.sh", "stop")

			for i := range repeatCountSingle {
				fname := fmt.Sprintf("bench_files/policies/set%dNet%v/%dRules%dAtts", i, false, rules, att)
				if benchLatency {
					fname += "Latency"
				}
				benchFunc := func(bBench *testing.B) {
					if _, err := os.Stat(fname); errors.Is(err, os.ErrNotExist) {
						buildPolicyCsv(fname, random, rules, newAttributes(random, att/2), newAttributes(random, att/2), false)
					}

					enforcer, err := casbin.NewSyncedEnforcer("bench_files/bench_model.conf", fname+".csv")
					if err != nil {
						log.Fatalln(err)
					}

					cSend := make(chan *ReadPacket, 1)
					cRcv := make(chan *ReadPacket, 1)

					serverEndpoint := newBenchmarkEndpoint(cSend, cRcv, se)
					defer serverEndpoint.Close()

					subAttHandler := NewGenericAttributeHandler(logger, newReqAttrs(random, att/2))
					objAttHandler := NewGenericAttributeHandler(logger, newReqAttrs(random, att/2))

					paacEndpoint := NewPAACEndPoint(logger, enforcer, serverEndpoint, netAttHandler, subAttHandler, objAttHandler)
					paacEndpoint.Start(runtime.NumCPU(), runtime.NumCPU())
					defer paacEndpoint.Close()

					requests := make([]*ReadPacket, bBench.N)
					for i := range bBench.N {
						id := strconv.Itoa(i)
						err := subAttHandler.Put(id, newReqAttrs(random, att/2))
						if err != nil {
							log.Fatalln("Failed putting subject attribute"+": ", err)
						}
						err = objAttHandler.Put(id, newReqAttrs(random, att/2))
						if err != nil {
							log.Fatalln("Failed putting object attribute"+": ", err)
						}

						requests[i] = buildRequestReadPacketNoPath(
							baseReadPacket,
							&PAACRequest{
								ClientID:   id,
								ObjectID:   id,
								AccessType: "read",
							})
					}

					bBench.ResetTimer()

					if !benchLatency {
						go func() {
							for i := range bBench.N {
								cSend <- requests[i]
							}
							close(cSend)
						}()
						for range bBench.N {
							<-cRcv
						}

					} else {
						for i := range bBench.N {
							cSend <- requests[i]
							<-cRcv
						}
						close(cSend)
					}
					<-netAttHandler.closed
					netAttHandler.cleanupQuit = make(chan bool)
					netAttHandler.closed = make(chan bool)
				}
				b.Run(name, benchFunc)
			}

		}
	}
}
