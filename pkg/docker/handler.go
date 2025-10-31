package docker

import (
	"aube/pkg/controlplane"
	"aube/pkg/util"
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	uuid2 "github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const (
	TmpDir           = "./tmp"
	containerTimeout = 1
)

type dockerHandler struct {
	name       string
	threads    int
	client     *client.Client
	filePath   string
	uniqueName string
	network    string
	// Define how many containers per function -> Users per function
	containers []string
	handlerIPs []string
}

type DockerBackend struct {
	client *client.Client
}

func (db *DockerBackend) Create(name string, filedir string) (controlplane.Handler, error) {
	uuid, err := uuid2.NewRandom()
	if err != nil {
		return nil, err
	}

	// A single handler represent a single function -> maybe one
	dh := &dockerHandler{
		name:       name,
		threads:    1,
		client:     db.client,
		containers: make([]string, 0), //unbounded to enable scaling each fn
		handlerIPs: make([]string, 0), //
	}

	dh.uniqueName = name + "-" + uuid.String()
	log.Printf("creating function %s with unique name %s", name, uuid.String())

	// copy the Docker runtime into folder
	// cp runtimes/python/* <folder>
	dh.filePath = path.Join(TmpDir, dh.uniqueName)
	err = os.MkdirAll(dh.filePath, 0777)
	if err != nil {
		log.Printf("creating folder for function %s failed with error: %v", name, err)
		return nil, err
	}

	err = util.CopyDirFromEmbed(runtimes, path.Join(runtimesDir, "python"), dh.filePath)
	if err != nil {
		log.Printf("copying runtime files to folder %s failed with error: %v", dh.filePath, err)
		return nil, err
	}

	// copy the function into the folder
	// cp <file> <folder>/fn
	err = os.MkdirAll(path.Join(dh.filePath, "fn"), 0777)
	if err != nil {
		log.Printf("creating folder for function failed with err: %v", err)
		return nil, err
	}

	err = util.CopyAll(filedir, path.Join(dh.filePath, "fn"))
	if err != nil {
		log.Printf("copying function into folder failed with error: %v", err)
		return nil, err
	}

	// Test without a build.Dockerfile (no blob.tar.gz)
	opt := client.ImageBuildOptions{
		Tags:       []string{dh.uniqueName},
		Remove:     true,
		Dockerfile: "Dockerfile",
		Labels: map[string]string{
			"aubefaas-function": dh.name,
			"AubeFaaS":          "AubeFaaS-Test",
		},
	}

	r, err := db.client.ImageBuild(context.Background(), nil, opt)
	if err != nil {
		log.Printf("building image based on opt: %+v, with error: %v", opt, err)
		return nil, err
	}
	defer r.Body.Close()
	scanner := bufio.NewScanner(r.Body)
	for scanner.Scan() {
		log.Println("scanner: ", scanner.Text())
	}

	nwOpt := client.NetworkCreateOptions{
		Labels: map[string]string{
			"aubefaas-function": dh.name,
			"AubeFaaS":          "AubeFaaS-Test",
		},
	}

	// Create Network
	nw, err := db.client.NetworkCreate(context.Background(), dh.uniqueName, nwOpt)
	if err != nil {
		log.Printf("creating docker network failed with error: %v", err)
		return nil, err
	}

	dh.network = nw.ID
	log.Printf("create network %s, with id: %s", dh.uniqueName, nw.ID)

	containerCfg := &container.Config{
		Image: dh.uniqueName,
		Labels: map[string]string{
			"aubefaas-function": dh.name,
			"AubeFaaS":          "AubeFaaS-Test",
		},
	}

	hostCfg := &container.HostConfig{
		NetworkMode: container.NetworkMode(dh.uniqueName),
	}

	for i := 0; i < dh.threads; i++ {
		c, err := db.client.ContainerCreate(
			context.Background(),
			containerCfg,
			hostCfg,
			nil,
			nil,
			dh.uniqueName+fmt.Sprintf("-%d", i),
		)
		if err != nil {
			log.Printf("creating container failed with err: %v", err)
			return nil, err
		}

		log.Printf("created container: %s", c.ID)
		dh.containers = append(dh.containers, c.ID)
	}

	err = os.RemoveAll(dh.filePath)
	if err != nil {
		return nil, err
	}

	log.Println("removed folder", dh.filePath)

	return dh, nil

}

func (d *DockerBackend) Stop() error {
	//TODO implement me
	panic("implement me")
}

func (d dockerHandler) IPs() []string {
	//TODO implement me
	panic("implement me")
}

func (d dockerHandler) Start() error {
	//TODO implement me
	panic("implement me")
}

func (d dockerHandler) Destroy() error {
	//TODO implement me
	panic("implement me")
}

func (d dockerHandler) Logs() (io.Reader, error) {
	//TODO implement me
	panic("implement me")
}
