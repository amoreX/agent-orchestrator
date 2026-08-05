package postgres

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	clouddomain "github.com/aoagents/agent-orchestrator/backend/internal/cloud/domain"
)

// TestSessionSecurityPolicyRoundTripIntegration proves the per-sandbox security
// policy (mode + denied commands + auto-stop) persists on CreateSession and is
// carried, unchanged, to the worker launch spec and sandbox row it reads back.
func TestSessionSecurityPolicyRoundTripIntegration(t *testing.T) {
	databaseURL := os.Getenv("AO_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AO_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := Migrate(ctx, databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	account, err := store.EnsureAccount(ctx, uuid.NewString(), "Security policy round-trip")
	if err != nil {
		t.Fatalf("EnsureAccount() error = %v", err)
	}
	project, err := store.CreateProject(ctx, account.ID, CreateProjectInput{
		DisplayName:   "policy project",
		RepositoryURL: "https://example.com/acme/policy.git",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	deniedCommands := []string{"git push --force:*", "rm -rf:*"}
	created, err := store.CreateSession(ctx, account.ID, CreateSessionInput{
		IdempotencyKey:  uuid.NewString(),
		ProjectID:       project.ID,
		Kind:            "worker",
		Harness:         "claude",
		DisplayName:     "policy session",
		Branch:          "feature/policy",
		Prompt:          "do the thing",
		Resource:        clouddomain.DefaultResourceProfile(),
		Mode:            clouddomain.SandboxModeReadOnly,
		DeniedCommands:  deniedCommands,
		AutoStopMinutes: 60,
		Provider:        "docker",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if !created.Created {
		t.Fatalf("CreateSession() Created = false, want true on first insert")
	}

	// 1. The create result reflects the policy verbatim.
	if created.Session.Mode != clouddomain.SandboxModeReadOnly {
		t.Errorf("Session.Mode = %q, want %q", created.Session.Mode, clouddomain.SandboxModeReadOnly)
	}
	if !reflect.DeepEqual(created.Session.DeniedCommands, deniedCommands) {
		t.Errorf("Session.DeniedCommands = %v, want %v", created.Session.DeniedCommands, deniedCommands)
	}
	if created.Sandbox.AutoStopMinutes != 60 {
		t.Errorf("Sandbox.AutoStopMinutes = %d, want 60", created.Sandbox.AutoStopMinutes)
	}

	// 2. The worker launch spec (what the worker actually applies) carries it.
	spec, err := store.WorkerLaunchSpec(ctx, account.ID, created.Session.ID)
	if err != nil {
		t.Fatalf("WorkerLaunchSpec() error = %v", err)
	}
	if spec.Session.Mode != clouddomain.SandboxModeReadOnly {
		t.Errorf("WorkerLaunchSpec Session.Mode = %q, want %q", spec.Session.Mode, clouddomain.SandboxModeReadOnly)
	}
	if !reflect.DeepEqual(spec.Session.DeniedCommands, deniedCommands) {
		t.Errorf("WorkerLaunchSpec Session.DeniedCommands = %v, want %v", spec.Session.DeniedCommands, deniedCommands)
	}

	// 3. The persisted sandbox row carries the auto-stop cap.
	sandbox, err := store.GetSandbox(ctx, account.ID, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSandbox() error = %v", err)
	}
	if sandbox.AutoStopMinutes != 60 {
		t.Errorf("GetSandbox AutoStopMinutes = %d, want 60", sandbox.AutoStopMinutes)
	}

	// 4. Idempotent replay returns the same row (DeepEqual path over the slice field).
	replay, err := store.CreateSession(ctx, account.ID, CreateSessionInput{
		IdempotencyKey:  created.Command.IdempotencyKey,
		ProjectID:       project.ID,
		Kind:            "worker",
		Harness:         "claude",
		DisplayName:     "policy session",
		Branch:          "feature/policy",
		Prompt:          "do the thing",
		Resource:        clouddomain.DefaultResourceProfile(),
		Mode:            clouddomain.SandboxModeReadOnly,
		DeniedCommands:  deniedCommands,
		AutoStopMinutes: 60,
		Provider:        "docker",
	})
	if err != nil {
		t.Fatalf("CreateSession() replay error = %v", err)
	}
	if replay.Created {
		t.Errorf("CreateSession() replay Created = true, want false on idempotent replay")
	}
	if replay.Session.ID != created.Session.ID {
		t.Errorf("replay Session.ID = %q, want %q", replay.Session.ID, created.Session.ID)
	}
}
