package apiserver

import (
	"context"
	"errors"
	"net/http"

	publicapi "github.com/ArthurC02/skillhub/apps/platform/internal/api/gen"
)

// generatedHealth implements the single generated operation currently mounted
// in production. Embedding UnimplementedHandler satisfies the full contract,
// but the generated router is mounted only behind the exact GET /healthz
// pattern in router.go; no unimplemented operation is reachable.
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
		// The server options are static and generated. Failing at process startup
		// is safer than silently replacing a liveness endpoint with an ad-hoc path.
		panic("create generated health handler: " + err.Error())
	}
	return server
}
