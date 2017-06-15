// Docker Image Authorization Plugin.
// Allows docker images to be fetched from a list of authorized registries only.
// AUTHOR: Chaitanya Prakash N <cpdevws@gmail.com>
package main

import (
	"encoding/json"
	dockerapi "github.com/docker/docker/api"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-plugins-helpers/authorization"
	"log"
	"net/url"
	"strings"
)

// Image Authorization Plugin struct definition
type ImgAuthZPlugin struct {
	// Docker client
	client *dockerclient.Client
	// Map of authorized registries
	authorizedRegistries map[string]bool
	// Number of authorized registries
	numAuthorizedRegistries int
	// List of authorized registries as string
	authRegistriesAsString string
	// Map of authorized images
	authorizedImages map[string]bool
	// Number of authorized registries
	numAuthorizedImages int
	// List of authorized registries as string
	authImagesAsString string
}

// Returns the list of authorized registries as string
func authRegistries(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// Returns the list of authorized images as string
func authImages(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// Create a new image authorization plugin
func newPlugin(dockerHost string, registries map[string]bool, images map[string]bool) (*ImgAuthZPlugin, error) {
	client, err := dockerclient.NewClient(dockerHost, dockerapi.DefaultVersion, nil, nil)

	if err != nil {
		return nil, err
	}

	return &ImgAuthZPlugin{
		client:                  client,
		authorizedRegistries:    registries,
		authorizedImages:        images,
		numAuthorizedRegistries: len(registries),
		numAuthorizedImages:     len(images),
		authRegistriesAsString:  authRegistries(registries),
		authImagesAsString:      authImages(images)}, nil
}

// Returns true if there are any authorized registries configured.
// Otherwise, returns false
func (plugin *ImgAuthZPlugin) hasAuthorizedRegistries() bool {
	return (plugin.numAuthorizedRegistries > 0)
}

// Returns true if there are any authorized images configured.
// Otherwise, returns false
func (plugin *ImgAuthZPlugin) hasAuthorizedImages() bool {
	return (plugin.numAuthorizedImages > 0)
}

// Parses the docker client command to determine the requested registry used in the command.
// If a registry is used in the command (i.e. docker pull or docker run commands), then the registry url and true is returned.
// Otherwise, returns empty string and false.
func (plugin *ImgAuthZPlugin) processRequest(req authorization.Request, reqURL *url.URL) (string, string, bool) {

	registry := ""
	imagePath := ""
	image := ""

	// docker run
	if strings.HasSuffix(reqURL.Path, "/containers/create") {
		var config dockercontainer.Config
		json.Unmarshal(req.RequestBody, &config)
		imagePath = config.Image
	}

	// docker pull
	if strings.HasSuffix(reqURL.Path, "/images/create") {
		imagePath = reqURL.Query().Get("fromImage")
	}

	if len(imagePath) > 0 {
		// If no registry is specfied, assume it is the dockerhub!
		registry = "library"
		idx := strings.Index(imagePath, "/")
		if idx != -1 {
			registry = imagePath[0:idx]
			image = imagePath[idx:]
		}
		return registry, image, true
	}

	return registry, image, false
}

// Authorizes the docker client command.
// Non registry related commands are allowed by default.
// If the command uses a registry, the command is allowed only if the registry is authorized.
// Otherwise, the request is denied!
func (plugin *ImgAuthZPlugin) AuthZReq(req authorization.Request) authorization.Response {
	// Parse request and the request body
	reqURI, _ := url.QueryUnescape(req.RequestURI)
	reqURL, _ := url.ParseRequestURI(reqURI)

	// Find out the requested registry and whether or not a registry is present in the client command
	requestedRegistry, requestedImage, isRegistryCommand := plugin.processRequest(req, reqURL)

	// Docker command do not involve registries
	if isRegistryCommand == false {
		// Allowed by default!
		log.Println("[ALLOWED] Not a registry command:", req.RequestMethod, reqURL.String())
		return authorization.Response{Allow: true}
	}

	// There are no authorized registries.
	if plugin.hasAuthorizedRegistries() == false {
		// So, deny the request by default!
		log.Println("[DENIED] No authorized registries", req.RequestMethod, reqURL.String())
		return authorization.Response{Allow: false, Msg: "No authorized registries configured"}
	}

	if plugin.hasAuthorizedImages() == false {
		log.Println("[DENIED] No authorized images", req.RequestMethod, reqURL.String())
		return authorization.Response{Allow: false, Msg: "No authorized images configured"}
	}

	// Verify that registry requested is authorized
	registryAuthorized := plugin.authorizedRegistries[requestedRegistry]
	imageAuthorized := plugin.authorizedImages[requestedImage]

	if registryAuthorized && imageAuthorized {
		// Is an authorized registry: Allow!
		log.Println("[ALLOWED] Registry:"+requestedRegistry+", Image: "+requestedImage, req.RequestMethod, reqURL.String())
		return authorization.Response{Allow: true}
	}

	// Oops.. The requested registry is not authorized. Deny the request!
	log.Println("[DENIED] Registry:", requestedRegistry, req.RequestMethod, reqURL.String())
	return authorization.Response{Allow: false, Msg: "You can only use docker images from the following authorized registries: " + plugin.authRegistriesAsString}
}

// Authorizes the docker client response.
// All responses are allowed by default.
func (plugin *ImgAuthZPlugin) AuthZRes(req authorization.Request) authorization.Response {
	// Allowed by default.
	return authorization.Response{Allow: true}
}
