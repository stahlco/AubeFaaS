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

The **Control Plane** is the core of our platform. It fully manages the upload, deletion and scale out/in of functions through two external endpoints: `/upload` and `/delete` and an internal endpoint: `/scale`.
Internally the **Control Plane** will be called by the **Reverse Proxy** when a function needs scale due to multiple tenants accessing it simultaneously. Our design ensures that each user utilizes a single Docker-Container, as running multiple streams within a container is undesireable as they may interfere with each other when writing to disk or memory.
As already said, the **Control Plane** is also responsible for the initial creation of a function, which means the creation of a function handler and the initial set of threads (containers). Once the creation is complete, it informs the **Reverse Proxy** of the available IP addresses for that specfic function.
When the **Reverse Proxy** calls the Control Plane's `/scale` endpoint, the **Control Plane** spawns `n` new containers for the specified function. It then responds with the IP addresses of the newly created containers, which are added to the Reverse Proxy's routing table.

#### Reverse Proxy

---

