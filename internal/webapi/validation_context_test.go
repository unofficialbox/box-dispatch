package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

type guardedFixture struct {
	mu       sync.Mutex
	plan     config.SolutionPlan
	settings config.ConnectionSettings
	store    *testRunStore
	manager  *runManager
}

func newGuardedFixture(t *testing.T, validate, deploy runExecutor) *guardedFixture {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dispatch.bcl"), []byte("configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &guardedFixture{plan: config.SolutionPlan{Name: "Guard fixture", TemplateID: "clm", Strategy: "reuse", PackagePath: root, Components: []string{"box", "salesforce"}}, store: &testRunStore{}}
	f.settings = f.settings.UpsertBoxConnection(config.BoxAppConnection{ID: "box-a", ClientID: "box-client", SubjectType: "user", SubjectID: "123", Identity: "box@example.test", Enterprise: "enterprise-a", RefreshToken: "private-original"}, true)
	f.settings = f.settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{ID: "sf-a", OrgID: "org-a", Username: "sf@example.test", InstanceURL: "https://org.example.test", ClientID: "sf-client", RefreshToken: "private-sf"}, true)
	if validate == nil {
		validate = func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
			return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusMissing, Deployable: true}}, nil
		}
	}
	if deploy == nil {
		deploy = func(_ context.Context, _ config.SolutionPlan, items []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
			return items, nil
		}
	}
	f.manager = newRunManagerWithStore(validate, deploy, time.Now, f.store)
	f.bind(f.manager)
	return f
}

func (f *guardedFixture) bind(manager *runManager) {
	manager.contextReader = func() (config.SolutionPlan, config.ConnectionSettings, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.plan, config.CloneConnectionSettings(f.settings), nil
	}
	manager.fingerprint = fingerprintValidationContext
}

func (f *guardedFixture) update(update shellstate.ConnectionUpdate) error {
	_, err := f.manager.guardConnectionUpdate(update, func(change shellstate.ConnectionUpdate) (config.ConnectionSettings, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		value, err := change(config.CloneConnectionSettings(f.settings))
		if err == nil {
			f.settings = config.CloneConnectionSettings(value)
		}
		return value, err
	})
	return err
}

func (f *guardedFixture) validate(t *testing.T) string {
	t.Helper()
	run, err := f.manager.startValidation(config.SolutionPlan{})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, f.manager, run.ID)
	response, _ := f.manager.response(run.ID)
	if response.Status != runCompleted {
		t.Fatalf("validation failed: %s", response.Status)
	}
	return run.ID
}

func TestValidationFingerprintBindsInputsButIgnoresTokenAndDisplayRefresh(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	baseline, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil {
		t.Fatal(err)
	}
	changed := config.CloneConnectionSettings(f.settings)
	changed.BoxConnections[0].RefreshToken = "private-rotated"
	changed.BoxConnections[0].Alias = "Renamed display"
	changed.BoxConnections[0].VerifiedAt = "later"
	changed.SalesforceOrgs[0].AccessToken = "private-access"
	changed.SalesforceOrgs[0].Status = "Checked"
	equivalent, err := fingerprintValidationContext(f.plan, changed)
	if err != nil || equivalent != baseline {
		t.Fatal("same-target refresh invalidated its fingerprint")
	}
	for _, mutate := range []func(*config.SolutionPlan, *config.ConnectionSettings){
		func(plan *config.SolutionPlan, _ *config.ConnectionSettings) { plan.Strategy = "create_new" },
		func(_ *config.SolutionPlan, settings *config.ConnectionSettings) {
			settings.BoxConnections[0].SubjectID = "another-user"
		},
		func(_ *config.SolutionPlan, settings *config.ConnectionSettings) {
			settings.SalesforceOrgs[0].OrgID = "another-org"
		},
	} {
		plan, settings := f.plan, config.CloneConnectionSettings(f.settings)
		mutate(&plan, &settings)
		got, err := fingerprintValidationContext(plan, settings)
		if err != nil || got == baseline {
			t.Fatal("changed context retained its fingerprint")
		}
	}
	if err := os.WriteFile(filepath.Join(f.plan.PackagePath, "dispatch.bcl"), []byte("changed run ID/configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil || updated == baseline {
		t.Fatal("package content drift was missed")
	}
	if err := os.WriteFile(filepath.Join(f.plan.PackagePath, ".env.production"), []byte("VITE_BUILD_MODE=changed"), 0600); err != nil {
		t.Fatal(err)
	}
	envUpdated, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil || envUpdated == updated {
		t.Fatal("package build environment drift was missed")
	}
	if err := os.Symlink("dispatch.bcl", filepath.Join(f.plan.PackagePath, "linked-input")); err != nil {
		t.Fatal(err)
	}
	if _, err := fingerprintValidationContext(f.plan, f.settings); err == nil {
		t.Fatal("linked input did not fail closed")
	}
}

func TestValidationAndDeploymentLeasesBlockChangesPreserveReplayAndAllowTokenRefresh(t *testing.T) {
	validationGate, deploymentGate := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f := newGuardedFixture(t, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		<-validationGate
		return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusMissing, Deployable: true}}, nil
	}, func(_ context.Context, _ config.SolutionPlan, items []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		calls.Add(1)
		<-deploymentGate
		return items, nil
	})
	run, err := f.manager.startValidation(config.SolutionPlan{})
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range []chan struct{}{validationGate, deploymentGate} {
		if _, err := f.manager.beginContextMutation(); err == nil {
			t.Fatal("active run allowed package mutation")
		}
		if _, err := f.manager.startValidation(config.SolutionPlan{}); err == nil {
			t.Fatal("active run allowed another validation")
		}
		if err := f.update(func(settings config.ConnectionSettings) (config.ConnectionSettings, error) {
			settings.BoxConnections[0].SubjectID = "changed"
			return settings, nil
		}); err == nil {
			t.Fatal("active run allowed target mutation")
		}
		if err := f.update(func(settings config.ConnectionSettings) (config.ConnectionSettings, error) {
			settings.BoxConnections[0].AccessToken = "rotated-access"
			return settings, nil
		}); err != nil {
			t.Fatalf("same-target refresh: %v", err)
		}
		close(gate)
		if gate == validationGate {
			waitForRun(t, f.manager, run.ID)
			accepted, err := f.manager.startDeployment(run.ID)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := f.manager.startDeployment(run.ID)
			if err != nil || replay.ID != accepted.ID {
				t.Fatal("active claim replay changed execution")
			}
		}
	}
	f.manager.mu.RLock()
	deploymentID := f.manager.runs[run.ID].deploymentRunID
	f.manager.mu.RUnlock()
	waitForRun(t, f.manager, deploymentID)
	if calls.Load() != 1 {
		t.Fatal("deployment executed more than once")
	}
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
	replay, err := f.manager.startDeployment(run.ID)
	if err != nil || replay.ID != deploymentID {
		t.Fatal("post-mutation replay changed execution")
	}
	data, _ := json.Marshal(f.store.runs)
	if strings.Contains(string(data), "private-original") || strings.Contains(string(data), "rotated-access") || strings.Contains(string(data), "sf@example.test") {
		t.Fatal("validation history disclosed target or credential fields")
	}
}

func TestInvalidationIsDurableBeforePartialMutationAndSurvivesRestart(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	id := f.validate(t)
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	if !f.store.runs[0].ValidationInvalidated {
		t.Fatal("write began without persisted invalidation")
	}
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("claim entered an outstanding mutation")
	}
	// Simulate a partially applied write whose operation then fails.
	if err := os.WriteFile(filepath.Join(f.plan.PackagePath, "dispatch.bcl"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	release()
	restored := newRunManagerWithStore(f.manager.validate, f.manager.deploy, time.Now, f.store)
	f.bind(restored)
	if _, err := restored.startDeployment(id); err == nil || !strings.Contains(err.Error(), "Validate again") {
		t.Fatalf("restored invalidation: %v", err)
	}
	f.manager = restored
	fresh := f.validate(t)
	accepted, err := restored.startDeployment(fresh)
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, restored, accepted.ID)
}

func TestFingerprintDriftLegacyAndUnreadableInputsRejectBeforeExecution(t *testing.T) {
	var calls atomic.Int32
	f := newGuardedFixture(t, nil, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		calls.Add(1)
		return nil, nil
	})
	id := f.validate(t)
	if err := os.WriteFile(filepath.Join(f.plan.PackagePath, "dispatch.bcl"), []byte("outside edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("changed files allowed deployment")
	}
	if err := os.RemoveAll(f.plan.PackagePath); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("missing inputs allowed deployment")
	}
	f.manager.mu.Lock()
	f.manager.runs[id].contextVersion = 0
	f.manager.mu.Unlock()
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("legacy unbound validation allowed deployment")
	}
	if calls.Load() != 0 {
		t.Fatal("rejected context invoked its executor")
	}
}

func TestFailedRunReleasesLeaseAndValidationSaveFailureDoesNotStartWork(t *testing.T) {
	f := newGuardedFixture(t, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		return nil, errors.New("failed validation")
	}, nil)
	run, err := f.manager.startValidation(config.SolutionPlan{})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, f.manager, run.ID)
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
	var calls atomic.Int32
	failing := &failingClaimStore{fail: true}
	manager := newRunManagerWithStore(func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		calls.Add(1)
		return nil, nil
	}, nil, time.Now, failing)
	f.bind(manager)
	if _, err := manager.startValidation(config.SolutionPlan{}); err == nil {
		t.Fatal("failed validation save accepted work")
	}
	if calls.Load() != 0 {
		t.Fatal("failed save invoked validation")
	}
	failing.fail = false
	next, err := manager.startValidation(config.SolutionPlan{})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, manager, next.ID)
	if calls.Load() != 1 {
		t.Fatal("storage recovery did not start one validation")
	}
}

func TestFailedInvalidationStopsWriterAndRetriesDurably(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	id := f.validate(t)
	store := &failingClaimStore{testRunStore: *f.store, fail: true}
	f.manager.mu.Lock()
	f.manager.store = store
	f.manager.mu.Unlock()
	if release, err := f.manager.beginContextMutation(); err == nil {
		release()
		t.Fatal("failed invalidation allowed a write")
	}
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("failed invalidation left validation claimable in memory")
	}
	f.manager.mu.Lock()
	store.fail = false
	f.manager.mu.Unlock()
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !store.runs[0].ValidationInvalidated {
		t.Fatal("retry omitted durable invalidation")
	}
	restored := newRunManagerWithStore(f.manager.validate, f.manager.deploy, time.Now, store)
	f.bind(restored)
	if _, err := restored.startDeployment(id); err == nil {
		t.Fatal("restart accepted invalid validation")
	}
}

type failingContextStore struct{ testRunStore }

func (s *failingContextStore) Save(runs []persistedRun) error {
	for _, run := range runs {
		if run.Action == runActionValidate && run.Status == runCompleted && run.ContextFingerprint != "" {
			return errors.New("context persistence failed")
		}
	}
	return s.testRunStore.Save(runs)
}

func TestCompletedValidationIsNotClaimableWhenContextSaveFails(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	store := &failingContextStore{}
	f.manager.store = store
	run, err := f.manager.startValidation(config.SolutionPlan{})
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, f.manager, run.ID)
	response, _ := f.manager.response(run.ID)
	if response.Status != runFailed {
		t.Fatalf("failed context save status = %s", response.Status)
	}
	if _, err := f.manager.startDeployment(run.ID); err == nil {
		t.Fatal("unsaved context was claimable")
	}
	restored := newRunManagerWithStore(f.manager.validate, f.manager.deploy, time.Now, store)
	f.bind(restored)
	if _, err := restored.startDeployment(run.ID); err == nil {
		t.Fatal("restart accepted unsaved context")
	}
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestConcurrentFirstClaimAndMutationHaveOneWinner(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		gate := make(chan struct{})
		f := newGuardedFixture(t, nil, func(_ context.Context, _ config.SolutionPlan, items []lifecycle.Item, _ func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
			<-gate
			return items, nil
		})
		id := f.validate(t)
		start := make(chan struct{})
		claim := make(chan error, 1)
		mutation := make(chan error, 1)
		go func() { <-start; _, err := f.manager.startDeployment(id); claim <- err }()
		go func() {
			<-start
			release, err := f.manager.beginContextMutation()
			if err == nil {
				release()
			}
			mutation <- err
		}()
		close(start)
		claimErr, mutationErr := <-claim, <-mutation
		close(gate)
		if (claimErr == nil) == (mutationErr == nil) {
			t.Fatalf("claim/mutation outcomes = %v / %v", claimErr, mutationErr)
		}
		if claimErr == nil {
			f.manager.mu.RLock()
			deploymentID := f.manager.runs[id].deploymentRunID
			f.manager.mu.RUnlock()
			waitForRun(t, f.manager, deploymentID)
		}
	}
}

func TestClaimReplayAfterRestartIgnoresNewContextAndUnsupportedContextFailsClosed(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	id := f.validate(t)
	accepted, err := f.manager.startDeployment(id)
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, f.manager, accepted.ID)
	if err := os.RemoveAll(f.plan.PackagePath); err != nil {
		t.Fatal(err)
	}
	restored := newRunManagerWithStore(f.manager.validate, f.manager.deploy, time.Now, f.store)
	f.bind(restored)
	replay, err := restored.startDeployment(id)
	if err != nil || replay.ID != accepted.ID {
		t.Fatalf("restart replay = %s / %v", replay.ID, err)
	}
	f2 := newGuardedFixture(t, nil, nil)
	id2 := f2.validate(t)
	f2.manager.mu.Lock()
	f2.manager.runs[id2].contextVersion = validationContextVersion + 1
	f2.manager.saveLocked()
	f2.manager.mu.Unlock()
	unsupported := newRunManagerWithStore(f2.manager.validate, f2.manager.deploy, time.Now, f2.store)
	f2.bind(unsupported)
	if _, err := unsupported.startDeployment(id2); err == nil {
		t.Fatal("unsupported context schema accepted")
	}
}

func TestPackageHTTPReservationBlocksClaimsConnectionsAndAsyncWorkflows(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	entered, finish := make(chan struct{}), make(chan struct{})
	handler := NewHandlerWithOptions(ServerOptions{
		Runs:      f.manager,
		PlanStore: func() (config.SolutionPlan, error) { f.mu.Lock(); defer f.mu.Unlock(); return f.plan, nil },
		ConnectionStore: func() (config.ConnectionSettings, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return config.CloneConnectionSettings(f.settings), nil
		},
		ConnectionSaver: func(settings config.ConnectionSettings) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.settings = config.CloneConnectionSettings(settings)
			return nil
		},
		Templates: func() ([]packageTemplate, error) { return []packageTemplate{{ID: "clm", Name: "Contracts"}}, nil },
		PackageAssembler: func(packageTemplate, []string, string) (config.SolutionPlan, error) {
			close(entered)
			<-finish
			return config.SolutionPlan{}, errors.New("partial package operation failed")
		},
	})
	id := f.validate(t)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/packages", strings.NewReader(`{"name":"Replacement","templateId":"clm","components":["box","salesforce"]}`)))
		close(done)
	}()
	<-entered
	if !f.store.runs[0].ValidationInvalidated {
		t.Fatal("HTTP package writer entered without invalidation")
	}
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/runs/" + id + "/deploy", ""},
		{http.MethodPost, "/api/runs", ""},
		{http.MethodDelete, "/api/connections/box/box-a", ""},
		{http.MethodPost, "/api/connections/box/oauth/start", ""},
		{http.MethodPost, "/api/connections/salesforce/oauth/start", `{}`},
		{http.MethodPost, "/api/salesforce/scratch-orgs", `{}`},
	} {
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, httptest.NewRequest(request.method, request.path, strings.NewReader(request.body)))
		if result.Code != http.StatusConflict {
			t.Errorf("%s %s = %d: %s", request.method, request.path, result.Code, result.Body.String())
		}
	}
	close(finish)
	<-done
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("partial write result = %d", response.Code)
	}
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("failed HTTP package write restored old validation")
	}
	f.validate(t)
}

func TestQueuedDeploymentRechecksContextBeforeProviderWork(t *testing.T) {
	var calls atomic.Int32
	f := newGuardedFixture(t, nil, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		calls.Add(1)
		return nil, nil
	})
	var reads atomic.Int32
	f.manager.fingerprint = func(plan config.SolutionPlan, settings config.ConnectionSettings) (string, error) {
		// An outside edit lands after the persisted claim, before execution's read.
		if reads.Add(1) == 4 {
			if err := os.WriteFile(filepath.Join(plan.PackagePath, "dispatch.bcl"), []byte("queued external drift"), 0600); err != nil {
				return "", err
			}
		}
		return fingerprintValidationContext(plan, settings)
	}
	id := f.validate(t)
	accepted, err := f.manager.startDeployment(id)
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, f.manager, accepted.ID)
	response, _ := f.manager.response(accepted.ID)
	if response.Status != runFailed || calls.Load() != 0 {
		t.Fatalf("preflight status/calls = %s/%d", response.Status, calls.Load())
	}
	replay, err := f.manager.startDeployment(id)
	if err != nil || replay.ID != accepted.ID {
		t.Fatalf("rejected executor claim replay = %s/%v", replay.ID, err)
	}
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestValidationBindsGeneratedPackageOutputsAndFailedDeploymentReleasesLease(t *testing.T) {
	var root string
	f := newGuardedFixture(t, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		if err := os.WriteFile(filepath.Join(root, "generated-ui.zip"), []byte("built bundle"), 0600); err != nil {
			return nil, err
		}
		return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusMissing, Deployable: true}}, nil
	}, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
		return nil, errors.New("deployment failed")
	})
	root = f.plan.PackagePath
	id := f.validate(t)
	accepted, err := f.manager.startDeployment(id)
	if err != nil {
		t.Fatal(err)
	}
	waitForRun(t, f.manager, accepted.ID)
	response, _ := f.manager.response(accepted.ID)
	if response.Status != runFailed {
		t.Fatal("provider failure was not recorded")
	}
	release, err := f.manager.beginContextMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
	fresh := f.validate(t)
	if err := os.WriteFile(filepath.Join(root, "generated-ui.zip"), []byte("different bundle"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.manager.startDeployment(fresh); err == nil {
		t.Fatal("generated bundle drift accepted")
	}
}

type unreadableGuardStore struct{ saves atomic.Int32 }

func (s *unreadableGuardStore) Load() ([]persistedRun, error) {
	return nil, errors.New("corrupt history")
}
func (s *unreadableGuardStore) Save([]persistedRun) error { s.saves.Add(1); return nil }

func TestUnreadableHistoryFailsClosedWithoutOverwritingClaimLedger(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	store := &unreadableGuardStore{}
	manager := newRunManagerWithStore(f.manager.validate, f.manager.deploy, time.Now, store)
	f.bind(manager)
	if _, err := manager.startValidation(config.SolutionPlan{}); err == nil {
		t.Fatal("unreadable history allowed validation")
	}
	if _, err := manager.startDeployment("lost-claim"); err == nil {
		t.Fatal("unreadable history accepted a claim")
	}
	if release, err := manager.beginContextMutation(); err == nil {
		release()
		t.Fatal("unreadable history allowed context mutation")
	}
	if store.saves.Load() != 0 {
		t.Fatal("unreadable claim ledger was overwritten")
	}
}

func TestBoxTargetEnvironmentOverridesAreBoundWithoutTokenValues(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	for _, key := range []string{"BOX_PARENT_FOLDER_ID", "BOX_WORKSPACE_NAME", "BOX_ALLOW_ROOT_FOLDER"} {
		t.Setenv(key, "")
	}
	baseline, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOX_REFRESH_TOKEN", "changed-private-token")
	refreshed, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil || refreshed != baseline {
		t.Fatal("token environment changed target fingerprint")
	}
	for _, entry := range []struct{ key, value string }{{"BOX_PARENT_FOLDER_ID", "123"}, {"BOX_WORKSPACE_NAME", "Different workspace"}, {"BOX_ALLOW_ROOT_FOLDER", "true"}} {
		t.Setenv(entry.key, entry.value)
		changed, err := fingerprintValidationContext(f.plan, f.settings)
		if err != nil || changed == baseline {
			t.Fatalf("%s target override was missed", entry.key)
		}
		t.Setenv(entry.key, "")
	}
}

func TestSelectedTargetRoundTripDoesNotReviveOldValidation(t *testing.T) {
	f := newGuardedFixture(t, nil, nil)
	id := f.validate(t)
	baseline, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"different-subject", "123"} {
		if err := f.update(func(settings config.ConnectionSettings) (config.ConnectionSettings, error) {
			settings.BoxConnections[0].SubjectID = subject
			return settings.HydrateBoxConnections(), nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	returned, err := fingerprintValidationContext(f.plan, f.settings)
	if err != nil || returned != baseline {
		t.Fatal("target did not return to original fingerprint")
	}
	if _, err := f.manager.startDeployment(id); err == nil {
		t.Fatal("A-to-B-to-A change revived old validation")
	}
	restored := newRunManagerWithStore(f.manager.validate, f.manager.deploy, time.Now, f.store)
	f.bind(restored)
	if _, err := restored.startDeployment(id); err == nil {
		t.Fatal("restart revived round-trip validation")
	}
}

func TestDelayedOAuthAndScratchCompletionCannotChangeActiveValidationTarget(t *testing.T) {
	for _, workflow := range []string{"box", "salesforce", "scratch"} {
		t.Run(workflow, func(t *testing.T) {
			t.Setenv("BOX_CLIENT_ID", "fixture-client")
			t.Setenv("BOX_CLIENT_SECRET", "fixture-secret")
			validationGate, entered, connectionGate := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseValidation, releaseConnection sync.Once
			t.Cleanup(func() {
				releaseValidation.Do(func() { close(validationGate) })
				releaseConnection.Do(func() { close(connectionGate) })
			})
			f := newGuardedFixture(t, func(context.Context, config.SolutionPlan, []lifecycle.Item, func(string, lifecycle.ProgressUpdate)) ([]lifecycle.Item, error) {
				<-validationGate
				return []lifecycle.Item{{Provider: "box", Status: lifecycle.StatusMissing, Deployable: true}}, nil
			}, nil)
			org := f.settings.SalesforceOrgs[0]
			org.DevHub = true
			org.AccessToken = "fixture-hub-token"
			f.settings = f.settings.UpsertSalesforceOrg(org, true)
			options := ServerOptions{
				Runs:      f.manager,
				PlanStore: func() (config.SolutionPlan, error) { f.mu.Lock(); defer f.mu.Unlock(); return f.plan, nil },
				ConnectionStore: func() (config.ConnectionSettings, error) {
					f.mu.Lock()
					defer f.mu.Unlock()
					return config.CloneConnectionSettings(f.settings), nil
				},
				ConnectionSaver: func(settings config.ConnectionSettings) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					f.settings = config.CloneConnectionSettings(settings)
					return nil
				},
				BoxOAuth: func(context.Context, boxconn.AuthorizationCodeRequest) (boxconn.TokenResponse, error) {
					close(entered)
					<-connectionGate
					return boxconn.TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"}, nil
				},
				BoxCheck: func(context.Context, config.ConnectionSettings) (config.VerifiedConnection, error) {
					return config.VerifiedConnection{Identity: "new@example.test", Account: "new-account", Enterprise: "new-enterprise"}, nil
				},
				SalesforceOAuth: func(context.Context, salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error) {
					close(entered)
					<-connectionGate
					return salesforceapi.TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", InstanceURL: "https://new.example.test"}, nil
				},
				SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
					return salesforceapi.OrgStatus{OrgID: "new-org", Username: "new@example.test", Status: "Active"}, nil
				},
				SalesforceDevHubCheck: func(context.Context, salesforceapi.Credential) (bool, error) { return true, nil },
				SalesforceCreate: func(context.Context, salesforceapi.Credential, salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error) {
					close(entered)
					<-connectionGate
					return salesforceapi.ScratchOrg{Alias: "New scratch", OrgID: "new-org", Username: "new@example.test", InstanceURL: "https://new.example.test", AccessToken: "new-access", RefreshToken: "new-refresh", Status: "Active"}, nil
				},
			}
			handler := NewHandlerWithOptions(options)
			before := config.DeploymentTargetFingerprint(f.settings)
			start := httptest.NewRecorder()
			done := make(chan struct{})
			var callback *httptest.ResponseRecorder
			var statusPath string
			if workflow == "scratch" {
				handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/salesforce/scratch-orgs", strings.NewReader(`{"alias":"New scratch","orgName":"New scratch","durationDays":7}`)))
				if start.Code != http.StatusAccepted {
					t.Fatalf("scratch start = %d: %s", start.Code, start.Body.String())
				}
				var job scratchJob
				if err := json.Unmarshal(start.Body.Bytes(), &job); err != nil {
					t.Fatal(err)
				}
				statusPath = "/api/salesforce/scratch-orgs/" + job.ID
			} else {
				handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/connections/"+workflow+"/oauth/start", strings.NewReader(`{"loginHost":"production","role":"org"}`)))
				if start.Code != http.StatusOK {
					t.Fatalf("OAuth start = %d: %s", start.Code, start.Body.String())
				}
				var job struct {
					ID           string `json:"id"`
					AuthorizeURL string `json:"authorizeUrl"`
				}
				if err := json.Unmarshal(start.Body.Bytes(), &job); err != nil {
					t.Fatal(err)
				}
				authorize, err := url.Parse(job.AuthorizeURL)
				if err != nil {
					t.Fatal(err)
				}
				statusPath = "/api/connections/" + workflow + "/oauth/" + job.ID
				callback = httptest.NewRecorder()
				go func() {
					handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/api/connections/"+workflow+"/oauth/callback?code=fixture-code&state="+authorize.Query().Get("state"), nil))
					close(done)
				}()
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("connection workflow did not reach barrier")
			}
			run, err := f.manager.startValidation(config.SolutionPlan{})
			if err != nil {
				t.Fatal(err)
			}
			releaseConnection.Do(func() { close(connectionGate) })
			if workflow != "scratch" {
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("callback blocked behind validation lease")
				}
				if !strings.Contains(callback.Body.String(), activeContextMessage) {
					t.Fatalf("callback omitted actionable lease guidance: %s", callback.Body.String())
				}
			}
			deadline := time.Now().Add(time.Second)
			failed := false
			for time.Now().Before(deadline) {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, statusPath, nil))
				if strings.Contains(response.Body.String(), `"status":"failed"`) {
					if workflow == "scratch" && (!strings.Contains(response.Body.String(), `"orgId":"new-org"`) || !strings.Contains(response.Body.String(), activeContextMessage)) {
						t.Fatalf("created scratch resource/recovery guidance was lost: %s", response.Body.String())
					}
					failed = true
					break
				}
				time.Sleep(time.Millisecond)
			}
			if !failed {
				t.Fatal("delayed connection workflow was not marked failed")
			}
			f.mu.Lock()
			after := config.DeploymentTargetFingerprint(f.settings)
			f.mu.Unlock()
			if after != before {
				t.Fatal("delayed callback replaced the active validation target")
			}
			releaseValidation.Do(func() { close(validationGate) })
			waitForRun(t, f.manager, run.ID)
		})
	}
}

func TestGuardedConnectionSavePreservesEnvironmentBootstrap(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("SALESFORCE_INSTANCE_URL", "https://bootstrap.example.test")
	t.Setenv("SALESFORCE_ACCESS_TOKEN", "fixture-bootstrap-access")
	t.Setenv("SALESFORCE_CLIENT_ID", "fixture-bootstrap-client")
	handler := NewHandlerWithOptions(ServerOptions{
		Runs: newRunManagerWithExecutors(nil, nil, time.Now),
		SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			return salesforceapi.OrgStatus{OrgID: "bootstrap-org", Username: "bootstrap@example.test", Status: "Active"}, nil
		},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap check = %d: %s", response.Code, response.Body.String())
	}
	saved, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if saved.SalesforceOrgID != "bootstrap-org" || saved.SalesforceClientID != "fixture-bootstrap-client" || saved.SalesforceAccessToken != "fixture-bootstrap-access" {
		t.Fatal("bootstrap state was not safely promoted")
	}
}
