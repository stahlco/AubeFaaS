# AubeFaaS

The Aube (French pronunciation: [ob]) is a river in France, a right tributary of the Seine, and a Function-as-a-Service Plattform optimized for Streaming Operations.

---
### Prerequisites:
- Go 1.23 >=
- Docker
- docker-mac-net-connect
- just

---
### Important Steps front up

```shell
# To Bridge the messages -> Only required on macOS (bridges from OS -> Linux VM in which Docker runs)
sudo docker-mac-net-connect
```

---
### Testing

```shell
cd pkg/docker/python
just start
```

---
