package profile

import "testing"

func TestProfileTarget(t *testing.T) {
	t.Parallel()

	if got := (Profile{ControllerURL: "http://127.0.0.1:9090"}).Target(); got != "http://127.0.0.1:9090" {
		t.Fatalf("unexpected controller target: %q", got)
	}
	if got := (Profile{UnixSocket: "/tmp/verge.sock"}).Target(); got != "unix:///tmp/verge.sock" {
		t.Fatalf("unexpected socket target: %q", got)
	}
}

func TestStoreUpsertValidatesExclusiveTargets(t *testing.T) {
	t.Parallel()

	store := &Store{}
	if err := store.Upsert(Profile{Name: "bad"}); err == nil {
		t.Fatal("expected missing target error")
	}
	if err := store.Upsert(Profile{Name: "bad", ControllerURL: "http://127.0.0.1:9090", UnixSocket: "/tmp/verge.sock"}); err == nil {
		t.Fatal("expected mutually exclusive target error")
	}
}
