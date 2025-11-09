# AubeFaaS

The Aube (French pronunciation: [ob]) is a river in France, a right tributary of the Seine, and a Function-as-a-Service Plattform optimized for Streaming Operations.

---
### Prerequisites:
- Go 1.23 >=
- Docker
- docker-mac-net-connect
- make

---
### Important Steps front up

```shell
# To Bridge the messages -> Only required on macOS (bridges from OS -> Linux VM in which Docker runs)
sudo docker-mac-net-connect
```

**Start the Control-Plane and the Reverse Proxy:**
```shell
# Please execute this command in the root-directory of the project
make start 
```

**Upload a Function**

```shell
sh ./scripts/upload.sh <folder-name> <name>
```
Example:
```shell
sh ./scripts/upload.sh ./test/fn test_function
```


---
# Architecture

![architecture.svg](resources/architecture.svg)

#### Control Plane

The **Control Plane** is the core of our platform. It fully manages the upload, deletion and scale out/in of functions through two external endpoints: `/upload` and `/delete` and an internal endpoint: `/scale`. Internally the **Control Plane** will be called by the **Reverse Proxy** when a function needs scale due to multiple tenants accessing it simultaneously. Our design ensures that each user utilizes a single Docker-Container, as running multiple streams within a container is undesireable as they may interfere with each other when writing to disk or memory. As already said, the **Control Plane** is also responsible for the initial creation of a function, which means the creation of a function handler and the initial set of threads (containers). Once the creation is complete, it informs the **Reverse Proxy** of the available IP addresses for that specfic function. When the **Reverse Proxy** calls the Control Plane's `/scale` endpoint, the **Control Plane** spawns `n` new containers for the specified function. It then responds with the IP addresses of the newly created containers, which are added to the Reverse Proxy's routing table.

#### Reverse Proxy

The **Reverse Proxy** is a lightweight, WebSocket-based reverse proxy designed to route client requests to dynamically managed funtion-threads (docker-containers). It acts as a central gateway that connects clients to function-specific backend instances and forwards WebSocket-Streams using Go's `io.Copy`-Function. At its core, it maintains a registry of functions and it's, available and already in use, threads (containers). Clients can send a request to `ws://<rproxy-addr>:8093/<function-name>`, and it will be fowarded to a available function container. The Reverse Proxy also manages the lifecycle of the containers, by scaling a function if the amount of available containers drop below a specific value (e.g. 1) or shutting down unused containers (e.g. 15 min unused). The Prototype isn't optimized in that manner.

#### Functions

---

