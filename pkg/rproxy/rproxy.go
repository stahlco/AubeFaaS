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
	hosts    map[string]string
	hl       sync.RWMutex
	upgrader websocket.Upgrader
}

func New() *RProxy {
	return &RProxy{
		hosts: make(map[string]string),
		upgrader: websocket.Upgrader{
			// Allows all origins to upgrade to a stream
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Add takes a container name and an ip-addr of a specific function
func (r *RProxy) Add(name string, ip string) error {
	r.hl.Lock()
	defer r.hl.Unlock()

	// if function exists, we should update!
	// if _, ok := r.hosts[name]; ok {
	// 	return fmt.Errorf("function already exists")
	// }

	r.hosts[name] = ip
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
	functionUrl, ok := r.hosts[functionName]
	if !ok {
		http.Error(w, "function not found", http.StatusNotFound)
		log.Printf("function not found: %s", functionName)
		return
	}

	log.Printf("chose functionUrl: %s", functionUrl)

	// Upgrade the HTTP-Request
	clientConn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("not able to upgrade request to websocket-stream with error: %v", err), http.StatusInternalServerError)
		log.Printf("not able to upgrade to ws-conn with err: %v", err)
		return
	}
	defer clientConn.Close()

	log.Printf("client successfully connected")

	functionConn, _, err := websocket.DefaultDialer.Dial(functionUrl, nil)
	if err != nil {
		log.Printf("failed to connect to the function: %v", err)
		clientConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(http.StatusInternalServerError, fmt.Sprintf("connecting to backend failed with err: %v", err)),
		)
		return
	}
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
		log.Printf("connection closed without an error")
	}
}
