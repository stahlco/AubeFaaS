package main

import (
	"aube/pkg/rproxy"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

const (
	UserAddr   = ":8093"
	ConfigAddr = ":8091"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("rproxy: ")

	proxy := rproxy.New()

	// Need a Config-Endpoint Server on Port :8091

	configServer := http.NewServeMux()

	configServer.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		b := new(bytes.Buffer)
		_, err := b.ReadFrom(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		str := b.String()

		d := struct {
			FunctionName string   `json:"name"`
			FunctionIPs  []string `json:"ips"`
		}{}

		err = json.Unmarshal([]byte(str), &d)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}

		if d.FunctionName[0] == '/' {
			d.FunctionName = d.FunctionName[1:]
		}

		if len(d.FunctionIPs) > 0 {
			err = proxy.Add(d.FunctionName, d.FunctionIPs)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		} else {
			err = proxy.Del(d.FunctionName)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	go func() {
		err := http.ListenAndServe(ConfigAddr, configServer)
		if err != nil {
			log.Fatalf("error listening to config")
		}
	}()

	// User Endpoint
	server := &http.Server{
		Addr:    UserAddr,
		Handler: proxy,
	}

	log.Printf("started on addr: %s", server.Addr)

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
