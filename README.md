# PAAC
This is a proof of concept implementation of `Path-Aware Access Control`. 

`PAAC` is an extension of Attribute-Based Access Control (ABAC) which allows the access control mechanism to collect and evaluate transit network attributes for remote access control requests sent via a path-aware underlying network.

The implementation is provided as a Go package which includes all necessary functionality to build and run simple `PAAC` server and client applications for the [SCION](https://github.com/scionproto/scion) internet architecture.

The server uses the [Casbin](https://github.com/casbin/casbin) authorization library to evaluate incoming access requests and supports most of its ABAC features, allowing for highly customizable access control policies.   
The server runs on a SCION host and waits for incoming access control requests sent by clients. It evaluates each request and sends a reply with the result of the access control decision back to the client.
The clients each run on their own (not necessarily different) SCION hosts and send requests to the server, waiting for each reply.
The server implementation supports any number of clients in parallel, but does not implement congestion control and request packets may be dropped as a result.

## Building `PAAC`
Build using `go build`. 
Requires Go 1.22.   
See the [go.mod](go.mod) file for the full list of required packages and their versions.

## Running an example
> [!NOTE]
> The implementation is designed to work on any endhost running SCION services (`sciond`, `dispatcher`), however it was only tested using a locally simulated SCION topology. Things may go wrong or require modifications when running on a real SCION deployment. 

We provide an example client/server application using the package in [main.go](main.go) which showcases the core functionality of `PAAC`. 
The example file includes explanations for each step. Individual types and functions within the package include additional documentation.

The example is made to run both the server and client on a locally simulated topology, defined by [topo/single_path_test.topo](topo/single_path_test.topo)
To run it:
1. Ensure the SCION Development Environment is installed. Refer to [the SCION documentation](https://docs.scion.org/en/latest/dev/setup.html) for setup instructions. 
To run `PAAC`, use a SCION version compatible with `https://github.com/scionproto/scion/releases/tag/v0.11.0`.
2. Start the local topology simulation for [topo/single_path_test.topo](topo/single_path_test.topo):  
  In the SCION directory, run:
    * `./scion.sh topology -c <path to single_path_test.topo`
    * `./scion.sh run`  
  To stop the simulation, run:  
    * `./scion.sh stop`  
  See [the SCION documentation](https://docs.scion.org/en/latest/dev/run.html) for details.
3. Run the example with `go run main.go`.
   This will 
    * Start a `PAAC` server configured with the Casbin policy and model defined in [examples/main_policy.csv](examples/main_policy.csv) and [examples/paac_model.conf](examples/paac_model.conf)
    * Start a `PAAC` client
    * Send a few access control requests over the simulated SCION network
    * Print the results
    * Stop `PAAC` components and exit


## Building your own `PAAC` applications
This section covers a brief overview of how to use the `PAAC` proof of concept package to design a server and client. Please refer to [main.go](main.go) for a concrete example.
### Configuring Casbin
As this implementation uses a Casbin enforcer to evaluate access requests, the first step is to define an appropriate Casbin model `.conf` file. 

Please refer to the [Casbin documentation](https://casbin.org/docs/overview) and [Casbin Go package](https://github.com/casbin/casbin) 
to learn about model definiton and different ABAC features. Not all features are well-documented.

[examples/paac_model.conf](examples/paac_model.conf) can be used as a starting point, but this implementation should
generally supports any ABAC model as long as requests are defined as `r = sub, obj, net, act`. 
We did not conduct extensive testing in this regard, so results may vary.   
For each request, `sub`, `obj` and `net` will be attribute maps (`map[string]any`) containing subject, object and network attributes respectively
and `act` must be a `string` defining the desired access type.

### Using the `PAAC` package
### Creating a Server
1. Create a `SCIONEndpoint` using `NewScionEndpoint`, with appropriate host and `sciond` 
addresses for the server.  
This endpoint will be used by the server to listen for incoming SCION packets containing the access
control requests and to send replies to the clients.

2. Create a Casbin `SyncedEnforcer` configured with an appropriate model and populate the policy with matching rules.
It is used to evaluate incoming requests.

3. Create subject and object attribute handlers with `NewGenericAttributeHandler` and 
populate them with IDs and attribute maps.  
`GenericAttributeHandler` includes methods `NewAttribute` and `RemoveAttribute` to modify the attribute set 
and `Put`, `Get` and `Delete` to manage entries.

4. Create a network attribute handler with `NewNetworkAttributeHandler` using the approproate `sciond` address.  
The `NetworkAttributeHandler` generates the following attributes for each received packet:
    * `SrcIA`: Source SCION address of the request
    * `MTU`: Smallest Maximum Transmission Unit over component links of the path
    * `Expiry`: Unix timestamp for the path expiry set by the control plane for the
    path
    * `Latency`: Latency of the network path, calculated as sum of latencies speci-
    fied in the metadata of each link on the path.
    * `Bandwidth`: Smallest bandwidth over component links of the path
    * `LinkType`: The highest `snet.LinkType` over component links of the path,
    specifying whether the path is contained in a local network or routed over
    the public internet
    * `Hops`: The total number of inter-AS hops in the path
    * `InternalHops`: The total number of intra-AS hops in the path
    * `ASList`: The list of ASes on the path

> [!NOTE]
> The ASList attribute can be used to check whether a specific AS is contained in the path by using the 
> `in` keyword in the Casbin matcher definition. 
> See the [casbin Go readme](https://github.com/casbin/casbin?tab=readme-ov-file#how-it-works) 
> for a basic example using `in`.

> [!NOTE]
> The above network attributes are generated using information contained in the
> packet header as well as metadata about the path returned by the scion daemon.
> Which attributes contain meaningful values and which correspond to a default
> value depends on what metadata is initialized by the control plane of the network.
>
> Currently, for a locally simulated topology, the following attributes are 
> initialized to non-default values: `SrcIA`, `MTU`, `Expiry`, `Hops`, `ASList`.

Additionally, the server application can set the `ExternalAttributeHandler` field to a `GenericAttributeHandler` 
and use it to manually manage additional attributes for specific network paths, using their `snet.Fingerprint`s 
as keys. These attributes are returned in the network attribute map together with the automatically generated ones.   

As the `NetworkAttributeHandler` has to retrieve network information from the `sciond` for each 
network path, an expensive operation, the methods `UseCache` and `SetCacheExpiry` can be used
to optionally enable caching of metadata for each path. Caching is disabled by default, but brings significant 
performance benefits and should be enabled (with appropriate expiry times) unless otherwise necessary.

5. Create the server `PAACEndPoint` using `NewPAACEndPoint` with the initialized components.
6. Call `Start` on the `PAACEndPoint`. This starts the server by spawning 
several communicating goroutines that handle individual components and functionality of `PAAC`, such as reading 
incoming packets, extracting paths, retrieving attributes, enforcing requests and sending replies.
The number of parallel Casbin enforcers and attribute retrieval routines can be manually specified.

### Creating a Client
1. Create a `SCIONEndpoint` using `NewScionEndpoint`, with appropriate host and `sciond` 
addresses for the client.  
The endpoint will be used to send requests to the server and wait for replies.

2. Create a `PAACClient` using `NewPAACClient` with the client `SCIONEndpoint`.

3. Use `RequestAccess` with appropriate server addresses to send an access request. This call blocks until a 
reply is received, without retries or checks for dropped packets.

> [!NOTE]
> All components of this implementation are designed to be thread-safe and 
usable while the components are running (e.g. attribute management, cache settings, policy updates...), 
but there may still be some concurrency bugs.


> [!NOTE]
> Make sure to call `Close()` on both the server and client endpoints when done.
> This will stop any running goroutines and ensure smooth termination.
> Once closed, endpoints must be recreated from scratch.

## Benchmarks
The implementation includes a number of benchmarks in [paac/paac_test.go](paac/paac_test.go), which can be
easily run using [paac/runner.sh](paac/runner.sh)(change the `scionDir` argument to match the SCION installation path). The outputs are written to [paac/results](paac/results), which already contains a set of precomputed benchmark results.
These outputs can be parsed into LaTeX tables and visualized in plots using [results/draw_graphs.py](paac/results/draw_graphs.py) which was developed for 
utility during development but is unpolished/undocumented and provided as-is.
