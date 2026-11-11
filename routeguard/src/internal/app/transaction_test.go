package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/app/mock"

	"github.com/ZeeeUs/RouteGuard/internal/policy"
	"github.com/ZeeeUs/RouteGuard/internal/store"
)

func simulatePowerLoss(t *testing.T, operation func()) {
	t.Helper()
	defer func() {
		if got := recover(); got != "power loss" {
			t.Errorf("expected injected power loss, got %v", got)
		}
	}()

	operation()
}

func TestRecoveryAtEveryTransactionBoundary(t *testing.T) {
	for _, phase := range []string{"before-kernel", "after-kernel", "after-save"} {
		t.Run(phase, func(t *testing.T) {
			directory := t.TempDir()
			a := fixtureApp(t, directory)
			files := a.Store
			old := store.State{Version: store.Version, Active: snapshot("192.0.2.10")}
			next := store.State{Version: store.Version, Generation: 1, Active: snapshot("192.0.2.20")}
			if err := files.Save(old); err != nil {
				t.Fatal(err)
			}

			installed := old.Active
			gomock.InOrder(
				a.firewall.EXPECT().Validate(gomock.Any(), next.Active).Return(nil),
				a.firewall.EXPECT().Install(gomock.Any(), next.Active).DoAndReturn(
					func(_ context.Context, target *policy.Snapshot) error {
						pending, err := files.Pending()
						if err != nil || !reflect.DeepEqual(pending, &next) {
							t.Fatalf("kernel changed before durable intent: %+v, %v", pending, err)
						}

						if phase == "before-kernel" {
							panic("power loss")
						}

						installed = target
						if phase == "after-kernel" {
							panic("power loss")
						}

						return nil
					}),
			)

			if phase == "after-save" {
				interrupted := mock.NewMockStateStore(gomock.NewController(t))
				gomock.InOrder(
					interrupted.EXPECT().Prepare(next).DoAndReturn(files.Prepare),
					interrupted.EXPECT().Save(next).DoAndReturn(func(state store.State) error {
						if err := files.Save(state); err != nil {
							t.Fatal(err)
						}

						panic("power loss")
					}),
				)
				a.Store = interrupted
			}

			simulatePowerLoss(t, func() { _ = a.publish(context.Background(), next, old) })

			// Recreate the application around the real on-disk journal. No UCI,
			// WAN or source calls are permitted during offline recovery.
			restarted := fixtureApp(t, directory)
			for range 2 {
				gomock.InOrder(
					restarted.firewall.EXPECT().Validate(gomock.Any(), next.Active).Return(nil),
					restarted.firewall.EXPECT().Install(gomock.Any(), next.Active).DoAndReturn(
						func(_ context.Context, target *policy.Snapshot) error {
							installed = target
							return nil
						}),
					restarted.scheduler.EXPECT().Update(gomock.Any(), "").Return(nil),
				)
			}
			if _, err := restarted.Execute(context.Background(), "restore"); err != nil {
				t.Fatal(err)
			}

			saved := restarted.loadState(t)
			pending, err := files.Pending()
			if err != nil || pending != nil || !reflect.DeepEqual(saved, next) || !reflect.DeepEqual(installed, next.Active) {
				t.Fatalf("recovery inconsistent: %+v, %v", saved, err)
			}
		})
	}
}

func TestDiskFailureRestoresPreviousRules(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	files := a.Store
	old := store.State{Version: store.Version, Active: snapshot("192.0.2.10")}
	next := store.State{Version: store.Version, Generation: 1, Active: snapshot("192.0.2.20")}
	if err := files.Save(old); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("disk full")
	storage := mock.NewMockStateStore(gomock.NewController(t))
	a.Store = storage
	gomock.InOrder(
		a.firewall.EXPECT().Validate(gomock.Any(), next.Active).Return(nil),
		storage.EXPECT().Prepare(next).DoAndReturn(files.Prepare),
		a.firewall.EXPECT().Install(gomock.Any(), next.Active).Return(nil),
		storage.EXPECT().Save(next).Return(failure),
		storage.EXPECT().Prepare(old).DoAndReturn(files.Prepare),
		a.firewall.EXPECT().Install(gomock.Any(), old.Active).Return(nil),
		storage.EXPECT().Save(old).DoAndReturn(files.Save),
		storage.EXPECT().Complete().DoAndReturn(files.Complete),
	)
	if err := a.publish(context.Background(), next, old); !errors.Is(err, failure) {
		t.Fatalf("lost original failure: %v", err)
	}

	saved, _, err := files.Load()
	pending, pendingErr := files.Pending()
	if err != nil || pendingErr != nil || pending != nil || !reflect.DeepEqual(saved, old) {
		t.Fatalf("rollback state inconsistent: %+v, %v, %v", saved, err, pendingErr)
	}
}

func TestValidationFailureDoesNotCreateJournal(t *testing.T) {
	a := fixtureApp(t, t.TempDir())
	a.Store = mock.NewMockStateStore(gomock.NewController(t))
	failure := errors.New("invalid nft")
	a.firewall.EXPECT().Validate(gomock.Any(), nil).Return(failure)

	if err := a.publish(context.Background(), store.State{}, store.State{}); !errors.Is(err, failure) {
		t.Fatalf("validation failure: %v", err)
	}
	// No Store calls or firewall installation are expected after validation fails.
}
