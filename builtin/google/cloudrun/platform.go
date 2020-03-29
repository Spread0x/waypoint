package cloudrun

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/api/option"
	run "google.golang.org/api/run/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mitchellh/devflow/builtin/docker"
	"github.com/mitchellh/devflow/sdk/component"
	"github.com/mitchellh/devflow/sdk/datadir"
)

// Platform is the Platform implementation for Google Cloud Run.
type Platform struct {
	config Config
}

// Config implements Configurable
func (p *Platform) Config() (interface{}, error) {
	return &p.config, nil
}

// DeployFunc implements component.Platform
func (p *Platform) DeployFunc() interface{} {
	return p.Deploy
}

// Deploy deploys an image to GCR.
func (p *Platform) Deploy(
	ctx context.Context,
	log hclog.Logger,
	src *component.Source,
	img *docker.Image,
	dir *datadir.Component,
) (*Deployment, error) {
	// Start building our deployment since we use this information
	result := &Deployment{
		Resource: &Deployment_Resource{
			Location: "us-central1",
			Project:  p.config.Project,
			Name:     src.App,
		},
	}

	apiService, err := run.NewService(ctx,
		option.WithEndpoint("https://"+result.Resource.Location+"-run.googleapis.com"),
	)
	if err != nil {
		return nil, status.Errorf(codes.Aborted, err.Error())
	}

	// Our service we'll be creating
	service := &run.Service{
		ApiVersion: "serving.knative.dev/v1",
		Kind:       "Service",
		Metadata: &run.ObjectMeta{
			Name: result.Resource.Name,
			Annotations: map[string]string{
				"devflow": "1",
			},
		},

		Spec: &run.ServiceSpec{
			Template: &run.RevisionTemplate{
				Spec: &run.RevisionSpec{
					Containers: []*run.Container{
						&run.Container{
							Image: img.Name(),
						},
					},
				},
			},
		},
	}

	// Create the service
	client := run.NewNamespacesServicesService(apiService)
	service, err = client.Create("namespaces/"+result.Resource.Project, service).
		Context(ctx).Do()
	if err != nil {
		return nil, status.Errorf(codes.Aborted, err.Error())
	}

	// Set the IAM policy so global traffic is allowed
	if err := p.setNoAuthPolicy(ctx, result, apiService); err != nil {
		return nil, err
	}

	// Poll the service and wait for completion
	service, err = p.pollServiceReady(ctx, log, result, apiService)
	if err != nil {
		return nil, err
	}

	// Now that the service is ready we can set the latest URL
	result.Url = service.Status.Url

	// If we have tracing enabled we just dump the full service as we know it
	// in case we need to look up what the raw value is.
	if log.IsTrace() {
		bs, _ := json.Marshal(service)
		log.Trace("service JSON", "json", base64.StdEncoding.EncodeToString(bs))
	}

	return result, nil
}

// pollServiceReady waits for the service to become ready.
func (p *Platform) pollServiceReady(
	ctx context.Context,
	log hclog.Logger,
	deployment *Deployment,
	apiService *run.APIService,
) (*run.Service, error) {
	log = log.With("service", deployment.Resource.Name)
	log.Info("waiting for cloud run service to be ready")
	client := run.NewNamespacesServicesService(apiService)
	for {
		log.Trace("querying service")
		service, err := client.Get(deployment.apiName()).Context(ctx).Do()
		if err != nil {
			return nil, status.Errorf(codes.Aborted, err.Error())
		}

		for _, cond := range service.Status.Conditions {
			if cond.Type != "Ready" {
				continue
			}

			log.Debug("ready status", "status", cond.Status)
			switch cond.Status {
			case "True":
				log.Info("service is ready")
				return service, nil

			case "False":
				return nil, status.Errorf(codes.Aborted, "service failed to get ready")
			}
		}

		time.Sleep(1 * time.Second)
	}
}

// setNoAuthPolicy sets the IAM policy on the deployment so that anyone
// can access it (no auth required).
func (p *Platform) setNoAuthPolicy(
	ctx context.Context,
	deployment *Deployment,
	apiService *run.APIService,
) error {
	client := run.NewProjectsLocationsServicesService(apiService)
	_, err := client.SetIamPolicy(deployment.apiResource(), &run.SetIamPolicyRequest{
		Policy: &run.Policy{
			Bindings: []*run.Binding{
				&run.Binding{
					Role:    "roles/run.invoker",
					Members: []string{"allUsers"},
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		return status.Errorf(codes.Aborted, err.Error())
	}

	return nil
}

// Config is the configuration structure for the Platform.
type Config struct {
	// Project is the project to deploy to.
	Project string `hcl:"project,attr"`

	// Unauthenticated, if set to true, will allow unauthenticated access
	// to your deployment. This defaults to true.
	Unauthenticated *bool `hcl:"unauthenticated,optional"`
}

var (
	_ component.Platform     = (*Platform)(nil)
	_ component.Configurable = (*Platform)(nil)
)