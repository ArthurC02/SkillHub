package apiserver

import (
	"context"
	"errors"
	"net/http"

	publicapi "github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/gen"
)

// Satisfies the generated interface in full, but router.go mounts this behind
// the exact GET /healthz pattern only, so no other operation is reachable.
type generatedHealth struct {
	publicapi.UnimplementedHandler
}

type rejectGeneratedSecurity struct{}

func (rejectGeneratedSecurity) HandleSessionCookie(
	context.Context,
	publicapi.OperationName,
	publicapi.SessionCookie,
) (context.Context, error) {
	return nil, errors.New("generated security is not an authorization boundary")
}

func (generatedHealth) GetHealth(context.Context) (*publicapi.Health, error) {
	return &publicapi.Health{Status: publicapi.HealthStatusOk}, nil
}

func newGeneratedHealthHandler() http.Handler {
	server, err := publicapi.NewServer(generatedHealth{}, rejectGeneratedSecurity{})
	if err != nil {

		panic("create generated health handler: " + err.Error())
	}
	return server
}
