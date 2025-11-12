package rproxy

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type RProxy struct {
	hosts    map[string]*Function
	hl       sync.RWMutex
	upgrader websocket.Upgrader
}

func (p *RProxy) GetHosts() map[string]*Function {
	return p.hosts
}

func New() *RProxy {
	return &RProxy{
		hosts: make(map[string]*Function),
		upgrader: websocket.Upgrader{
			// Allows all origins to upgrade to a stream
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Add takes a container name and an ip-addr of a specific function
func (r *RProxy) Add(name string, ips []string) error {
	log.Printf("received function %s now adding it with IPs : [%v]", name, ips)

	f := NewFunction(name, ips)

	r.hl.Lock()
	defer r.hl.Unlock()

	// if function exists, we should update!
	// if _, ok := r.hosts[name]; ok {
	// 	return fmt.Errorf("function already exists")
	// }

	r.hosts[name] = f
	return nil
}

func (r *RProxy) Del(name string) error {
	r.hl.Lock()
	defer r.hl.Unlock()

	if _, ok := r.hosts[name]; !ok {
		return fmt.Errorf("function not found")
	}

	delete(r.hosts, name)
	return nil
}

func (r *RProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log.Printf("received req: %v", req.URL)
	functionName := req.URL.Path

	if functionName == "" || functionName == "/" {
		http.Error(w, "function-name must include the name of the function", http.StatusBadRequest)
		log.Printf("receive rerquest must include the name of the function, received: %+v", req)
		return
	}

	if functionName[0] == '/' {
		functionName = functionName[1:]
	}

	// Get the function backend -> Could also be a map (but it's just a single addr -> no handler just a single IP)
	function, ok := r.hosts[functionName]
	if !ok {
		http.Error(w, "function not found", http.StatusNotFound)
		log.Printf("function not found: %s", functionName)
		return
	}

	log.Printf("chose functionUrl: %s", function)

	// Upgrade the HTTP-Request
	clientConn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("not able to upgrade request to websocket-stream with error: %v", err), http.StatusInternalServerError)
		log.Printf("not able to upgrade to ws-conn with err: %v", err)
		return
	}
	defer clientConn.Close()

	log.Printf("client successfully connected")

	// This call simultaneously "blocks" the container
	containerIP, err := function.getContainer()
	if err != nil {
		log.Printf("Not able to get a Container for the function")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("containerIP: %s", containerIP)
	functionConn, _, err := websocket.DefaultDialer.Dial(containerIP, nil)
	if err != nil {
		log.Printf("failed to connect to the function: %v", err)
		clientConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(http.StatusInternalServerError, fmt.Sprintf("connecting to backend failed with err: %v", err)),
		)
		return
	}
	// Free the container
	defer function.freeContainer(containerIP)
	defer functionConn.Close()

	log.Printf("connected to function-backend successfully")

	// Get underlying network connections for io.Copy
	clientNetConn := clientConn.NetConn()
	fnNetConn := functionConn.NetConn()

	log.Printf("the underlying connection are of Type: %T, %T", clientNetConn, fnNetConn)

	errChan := make(chan error, 2)

	// Client -> Function
	go func() {
		// Write -> WriteTo, Reader -> ReadFrom
		_, err := io.Copy(fnNetConn, clientNetConn)
		log.Printf("client to function stream closed: %v", err)
		errChan <- err
	}()

	// Function -> Client
	go func() {
		_, err := io.Copy(clientNetConn, fnNetConn)
		log.Printf("function to client stream closed: %v", err)
		errChan <- err
	}()

	err = <-errChan
	if err != nil {
		log.Printf("connection closed with error: %v", err)
	} else {
		// Connection closed, need to move the container to freeContainer
		log.Printf("connection closed without an error")
	}
}
