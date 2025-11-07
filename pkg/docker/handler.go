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
	"github.com/moby/go-archive"
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
	handler.filePath = path.Join(TmpDir, handler.uniqueName) // mkdir <folder>
	log.Printf("Create Folder: %s", handler.filePath)

	err = os.MkdirAll(handler.filePath, 0777)
	if err != nil {
		return nil, err
	}

	err = PrintFolderEntries(handler.filePath)
	if err != nil {
		log.Fatal(err)
	}

	//srcPath := path.Join(runtimesDir, "python")

	err = PrintFolderEntries("./")
	if err != nil {
		log.Fatal(err)
	}

	pwd, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	log.Printf("DEBUG: Current Directory %s", pwd)

	log.Printf("Executing: cp %s %s", path.Join(runtimesDir, "python/"), handler.filePath)
	err = util.CopyDirFromEmbed(runtimes, path.Join(runtimesDir, "python"), handler.filePath)
	if err != nil {
		log.Printf("copying embed filesystem (python-runtime) into function failed with err: %v", err)
		return nil, err
	}

	err = PrintFolderEntries(handler.filePath)
	if err != nil {
		log.Fatal(err)
	}

	err = PrintFolderEntries(handler.filePath)
	if err != nil {
		log.Fatal(err)
	}

	// Copy function-code into the runtime
	// cp <filedir> <folder>/fn

	functionFilePath := path.Join(handler.filePath, "fn")
	log.Printf("Create functionFilePath: %s", functionFilePath)

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

	err = PrintFolderEntries(handler.filePath)
	if err != nil {
		log.Fatal(err)
	}

	tar, err := archive.TarWithOptions(handler.filePath, &archive.TarOptions{})
	if err != nil {
		return nil, err
	}

	log.Printf("Created tar: %+v", tar)

	imageBuildOpts := client.ImageBuildOptions{
		Tags:       []string{handler.uniqueName},
		Dockerfile: "Dockerfile",
		Remove:     true,
		Labels: map[string]string{
			"AubeFaaS-Function": handler.uniqueName,
			"AubeFaaS-ID":       d.id,
		},
	}

	imageResp, err := handler.client.ImageBuild(context.Background(), tar, imageBuildOpts)
	if err != nil {
		log.Printf("Building failed with error: %v", err)
		return nil, err
	}
	defer imageResp.Body.Close()
	// Reading Body from Image Creation
	scanner := bufio.NewScanner(imageResp.Body)
	for scanner.Scan() {
		log.Println(scanner.Text())
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

	// Where do we add the IP?

	return nil
}

func PrintFolderEntries(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		log.Printf("DEBUG: %s contains: %s", dir, entry.Name())
	}
	return nil
}

// Add allows that we can scale-out, this function adds a single new container.
// So for adding several instances Add must be called the desired amount of times.
func (handler *dockerHandler) Add() (string, error) {

	prev := handler.containers

	err := createContainer(handler, 1)
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

func (handler *dockerHandler) StartContainer(name string) error {
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

func (handler *dockerHandler) Start() error {

	log.Printf("DEBUG: Starting Containers of hanler %+v", handler)
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

			log.Printf("successfully started container: %s", c)
		}(c)
	}

	wg.Wait()

	log.Printf("Successfully started all container")

	// get container IPs
	// docker inspect <container>
	for _, c := range handler.containers {
		insp, err := handler.client.ContainerInspect(context.Background(), c)
		if err != nil {
			log.Printf("inspecting container failed with error")
			return err
		}
		ip := insp.NetworkSettings.Networks[handler.uniqueName].IPAddress.String()
		log.Printf("IP-Address of Container %v is: %s", c, ip)
		handler.containerIPs = append(handler.containerIPs, ip)
	}

	log.Printf("FH: %v, This the list of ips fetched: %v", handler, handler.containerIPs)

	// add health-checks but entpoint does not exist

	return nil
}

func (handler *dockerHandler) Delete(name string) error {
	return fmt.Errorf("currently not implemented")
}

func (d DockerBackend) Stop() error {
	return fmt.Errorf("currently not implemented")
}

func (handler *dockerHandler) IPs() []string {
	log.Printf("DEBUG: Received request to give the List of IPs of FH: %v which is: %v", handler, handler.containerIPs)
	return handler.containerIPs
}

func (handler *dockerHandler) Destroy() error {
	// TODO
	return fmt.Errorf("currently not implemented")
}

func (handler *dockerHandler) Logs() (io.Reader, error) {
	return nil, fmt.Errorf("currently not implemented")
}
