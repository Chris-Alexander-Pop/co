package history

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLatestIsThePreviousBuild(t *testing.T) {
	b, err := loadPath(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	b.Put(Run{ID: "1", Name: "zen-kernel", Files: 1000, Duration: 100 * time.Second, Ended: time.Unix(100, 0)})
	b.Put(Run{ID: "2", Name: "zen-kernel", Files: 1100, Duration: 90 * time.Second, Ended: time.Unix(200, 0)})
	b.Put(Run{ID: "9", Name: "other", Files: 10, Duration: time.Second, Ended: time.Unix(300, 0)})
	got, ok := b.Latest("zen-kernel", "2")
	if !ok || got.ID != "1" || got.Files != 1000 {
		t.Fatalf("latest %+v ok=%v", got, ok)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := loadPath(b.path)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Has("1") || !again.Has("2") {
		t.Fatalf("saved %+v", again.Runs)
	}
}
