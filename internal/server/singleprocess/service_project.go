package singleprocess

import (
	"context"

	pb "github.com/hashicorp/waypoint/internal/server/gen"
	serverptypes "github.com/hashicorp/waypoint/internal/server/ptypes"
)

// TODO: test
func (s *service) UpsertProject(
	ctx context.Context,
	req *pb.UpsertProjectRequest,
) (*pb.UpsertProjectResponse, error) {
	result := req.Project
	if err := s.state.ProjectPut(result); err != nil {
		return nil, err
	}

	return &pb.UpsertProjectResponse{Project: result}, nil
}

// TODO: test
func (s *service) GetProject(
	ctx context.Context,
	req *pb.GetProjectRequest,
) (*pb.GetProjectResponse, error) {
	result, err := s.state.ProjectGet(req.Project)
	if err != nil {
		return nil, err
	}

	return &pb.GetProjectResponse{Project: result}, nil
}

// TODO: test
func (s *service) UpsertApplication(
	ctx context.Context,
	req *pb.UpsertApplicationRequest,
) (*pb.UpsertApplicationResponse, error) {
	// Get the project
	praw, err := s.state.ProjectGet(req.Project)
	if err != nil {
		return nil, err
	}

	// If the project has the application already then we're done.
	p := serverptypes.Project{Project: praw}
	if idx := p.App(req.Name); idx >= 0 {
		return &pb.UpsertApplicationResponse{Application: p.Applications[idx]}, nil
	}

	// Initialize a new app.
	app, err := s.state.AppPut(&pb.Application{
		Project: req.Project,
		Name:    req.Name,
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpsertApplicationResponse{Application: app}, nil
}
