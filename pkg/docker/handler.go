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
	"slices"
	"sync"

	uuid2 "github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const (
	TmpDir = "./tmp"
)

type DockerBackend struct {
	id     string
	client *client.Client
}

// Each dockerHandler represents a single function with n containers
type dockerHandler struct {
	name        string
	uniqueName  string // Determines Image and Network as well
	initThreads int
	maxThreads  int
	filePath    string
	// Docker specific stuff -> needed to create or remove containers
	client          *client.Client
	containers      []string
	containerIPs    []string
	network         string
	containerConfig *container.Config
	hostConfig      *container.HostConfig
}

func New(aubeFaaSID string) (*DockerBackend, error) {
	id := "AubeFaaS-" + aubeFaaSID

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &DockerBackend{
		id:     id,
		client: c,
	}, nil
}

// Create creates a new function with function-name: name in the file-directory
// filedir must include ./fn.py and ./requirements -> this will be loaded into to the container
// filedir would be: ./test/fn
func (d DockerBackend) Create(name string, filedir string, initThreads int, maxThreads int) (controlplane.Handler, error) {

	// Create a new unique function name
	uuid, err := uuid2.NewRandom()
	if err != nil {
		return nil, err
	}

	uniqueName := name + "-" + uuid.String()

	handler := &dockerHandler{
		name:         name,
		uniqueName:   uniqueName,
		client:       d.client,
		initThreads:  initThreads,
		maxThreads:   maxThreads,
		containers:   make([]string, 0, maxThreads),
		containerIPs: make([]string, 0, maxThreads),
	}

	// Copy the Docker-Runtime into a folder
	// cp runtimes/python/* ./tmp/<uniqueName>
	handler.filePath = path.Join(TmpDir, handler.uniqueName)
	err = os.MkdirAll(handler.filePath, 0777)
	if err != nil {
		return nil, err
	}

	srcPath := path.Join(runtimesDir, "python")
	err = util.CopyDirFromEmbed(runtimes, srcPath, handler.filePath)
	if err != nil {
		log.Printf("copying embed filesystem (python-runtime) into function failed with err: %v", err)
		return nil, err
	}

	// Copy function-code into the runtime
	// cp <filedir> <folder>/fn

	functionFilePath := path.Join(handler.filePath, "fn")
	err = os.MkdirAll(functionFilePath, 0777)
	if err != nil {
		log.Printf("creating folder for function failed with err: %v", err)
		return nil, err
	}

	err = util.CopyAll(filedir, functionFilePath)
	if err != nil {
		log.Printf("copying function code into fn-folder failed with err: %v", err)
		return nil, err
	}

	// Creating a Docker Image
	imageBuildOpts := client.ImageBuildOptions{
		Tags:       []string{handler.uniqueName},
		Dockerfile: "Dockerfile",
		Remove:     true,
		Labels: map[string]string{
			"AubeFaaS-Function": handler.uniqueName,
			"AubeFaaS-ID":       d.id,
		},
	}

	imageResp, err := handler.client.ImageBuild(context.Background(), nil, imageBuildOpts)
	if err != nil {
		return nil, err
	}
	defer imageResp.Body.Close()
	// Reading Body from Image Creation
	scanner := bufio.NewScanner(imageResp.Body)
	for scanner.Scan() {
		log.Printf("scanner" + scanner.Text())
	}

	networkOpts := client.NetworkCreateOptions{
		Labels: map[string]string{
			"AubeFaaS-Function": handler.name,
			"AubeFaaS-ID":       d.id,
		},
	}

	nw, err := handler.client.NetworkCreate(context.Background(), handler.uniqueName, networkOpts)
	if err != nil {
		return nil, err
	}

	handler.network = nw.ID

	containerConfig := &container.Config{
		Image: handler.uniqueName,
		Labels: map[string]string{
			"AubeFaaS-Function": handler.uniqueName,
			"AubeFaaS-ID":       d.id,
		},
	}

	handler.containerConfig = containerConfig

	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(handler.uniqueName),
	}

	handler.hostConfig = hostConfig

	err = createContainer(handler, initThreads)
	if err != nil {
		return nil, err
	}

	return handler, nil
}

func createContainer(handler *dockerHandler, amount int) error {

	if curr := len(handler.containers); (curr + amount) > handler.maxThreads {
		amount = handler.maxThreads - curr
		log.Printf("Not able to create more than %d container because it would exceed the upper resource bound", amount)
	}

	for i := 0; i < amount; i++ {
		idx := len(handler.containers)

		c, err := handler.client.ContainerCreate(
			context.Background(),
			handler.containerConfig,
			handler.hostConfig,
			nil,
			nil,
			handler.uniqueName+fmt.Sprintf("-%d", idx),
		)
		if err != nil {
			return err
		}
		handler.containers = append(handler.containers, c.ID)
	}

	return nil
}

// Add allows that we can scale-out, this function adds a single new container.
// So for adding several instances Add must be called the desired amount of times.
func (handler dockerHandler) Add() (string, error) {

	prev := handler.containers

	err := createContainer(&handler, 1)
	if err != nil {
		return "", err
	}

	curr := handler.containers

	slices.Sort(prev)
	slices.Sort(curr)

	var containerName string

	for i := 0; i < len(prev); i++ {
		if prev[i] != curr[i] {
			containerName = curr[i]
		}
	}
	containerName = curr[len(curr)-1]

	return containerName, err
}

func (handler dockerHandler) StartContainer(name string) error {
	wg := sync.WaitGroup{}

	wg.Add(1)

	errChan := make(chan error, 1)
	go func(name string) {
		defer wg.Done()
		err := handler.client.ContainerStart(context.Background(), name, client.ContainerStartOptions{})
		if err != nil {
			errChan <- err
			log.Printf("Not able to start container")
			return
		}
	}(name)
	wg.Wait()

	err := <-errChan
	if err != nil {
		return err
	}

	// get container IP
	insp, err := handler.client.ContainerInspect(context.Background(), name)
	if err != nil {
		log.Printf("not able to inspect container %s with err: %v", name, err)
		return err
	}
	ip := insp.NetworkSettings.Networks[handler.uniqueName].IPAddress.String()
	handler.containerIPs = append(handler.containerIPs, ip)

	// add health-check but no endpoint exists
	return nil
}

func (handler dockerHandler) Start() error {

	wg := sync.WaitGroup{}

	// Is this important?
	// errChan := make(chan error)
	for _, c := range handler.containers {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			err := handler.client.ContainerStart(context.Background(), c, client.ContainerStartOptions{})
			if err != nil {
				log.Printf("Error starting container: %v", err)
				return
			}

			log.Printf("successfully created container: %s", c)
		}(c)
	}

	wg.Wait()

	// get container IPs
	for _, c := range handler.containers {
		insp, err := handler.client.ContainerInspect(context.Background(), c)
		if err != nil {
			log.Printf("inspecting container failed with error")
			return err
		}
		ip := insp.NetworkSettings.Networks[handler.uniqueName].IPAddress.String()
		handler.containerIPs = append(handler.containerIPs, ip)
	}

	// add health-checks but entpoint does not exist

	return nil
}

func (handler dockerHandler) Delete(name string) error {
	return fmt.Errorf("currently not implemented")
}

func (d DockerBackend) Stop() error {
	return fmt.Errorf("currently not implemented")
}

func (handler dockerHandler) IPs() []string {
	return handler.containerIPs
}

func (handler dockerHandler) Destroy() error {
	// TODO
	return fmt.Errorf("currently not implemented")
}

func (handler dockerHandler) Logs() (io.Reader, error) {
	return nil, fmt.Errorf("currently not implemented")
}
