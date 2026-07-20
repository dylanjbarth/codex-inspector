package process

import (
	"os"
	"testing"
	"time"
)

func TestMetadataAtomicUserOnlyRoundTrip(t *testing.T) {
	run := t.TempDir()
	want := Metadata{InstanceID: "fake-instance", PID: 123, Port: 4567, ProtocolVersion: 1, AccessToken: "secret", FragmentToken: "fragment", StartupStage: "source_format", StartedAt: time.Unix(1, 0).UTC()}
	if err := Write(run, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(run)
	if err != nil {
		t.Fatal(err)
	}
	if got.InstanceID != want.InstanceID || got.AccessToken != want.AccessToken || got.StartupStage != want.StartupStage {
		t.Fatalf("got %+v", got)
	}
	info, _ := os.Stat(Path(run))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	if err = RemoveIfInstance(run, "other"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(Path(run)); err != nil {
		t.Fatal(err)
	}
	if err = RemoveIfInstance(run, want.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(Path(run)); !os.IsNotExist(err) {
		t.Fatal("metadata not removed")
	}
}
