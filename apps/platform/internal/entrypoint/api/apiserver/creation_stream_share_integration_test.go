package apiserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func openCreationStream(t *testing.T, c *client, sessionID string) (int, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/creation-sessions/"+sessionID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return resp.StatusCode, func() { cancel(); resp.Body.Close() }
}

func TestAWorkspaceOpensOnlyItsShareOfCreationStreams(t *testing.T) {
	a, _, _ := creationFixture(t)
	alice := a.login(t, "creation-stream-share-alice")
	bob := a.login(t, "creation-stream-share-bob")
	start := func(c *client) string {
		return creationPost(t, c, "/creation-sessions", map[string]any{
			"id": creationID(t), "message": "建立一個摘要 Skill", "budget_credits": 650,
		}, 200).ID
	}
	aliceSession, bobSession := start(alice), start(bob)

	for i := range creation.MaxStreamsPerWorkspace {
		status, closeStream := openCreationStream(t, alice, aliceSession)
		defer closeStream()
		if status != http.StatusOK {
			t.Fatalf("stream %d of %d got %d", i+1, creation.MaxStreamsPerWorkspace, status)
		}
	}
	status, closeOver := openCreationStream(t, alice, aliceSession)
	defer closeOver()
	if status != http.StatusTooManyRequests {
		t.Fatalf("stream %d got %d, want 429", creation.MaxStreamsPerWorkspace+1, status)
	}
	status, closeBob := openCreationStream(t, bob, bobSession)
	defer closeBob()
	if status != http.StatusOK {
		t.Fatalf("another workspace's stream got %d while this one was full", status)
	}
}
