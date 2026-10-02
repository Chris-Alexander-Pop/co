package progress

import "testing"

func TestFeedKernelLines(t *testing.T) {
	tr := New()
	tr.Feed("  CC      lib/is_single_threaded.o\n")
	tr.Feed("  AS      arch/x86/entry/vdso/vdso64/vsgx.o\n  CC [M]  sound/core/hwdep.o\n")
	if tr.File != "sound/core/hwdep.o" || tr.Verb != "CC [M]" {
		t.Fatalf("last %s %s", tr.Verb, tr.File)
	}
	if tr.Compiled() != 3 {
		t.Fatalf("compiled %d", tr.Compiled())
	}
	if p := tr.Percent(false); p <= 0 || p >= 100 {
		t.Fatalf("percent %d", p)
	}
	if tr.Percent(true) != 100 {
		t.Fatal(tr.Percent(true))
	}
}

func TestSplitChunk(t *testing.T) {
	tr := New()
	tr.Feed("  CC      ker")
	tr.Feed("nel/fork.o\n")
	if tr.File != "kernel/fork.o" || tr.Compiled() != 1 {
		t.Fatalf("%s %d", tr.File, tr.Compiled())
	}
}

func TestInstallPhase(t *testing.T) {
	tr := New()
	tr.Feed("  CC      init/main.o\n  INSTALL include/bpf/bpf.h\n  INSTALL out/lib/modules/foo.ko\n  DEPMOD  7.2.6\n")
	if tr.Compiled() != 2 {
		t.Fatalf("compiled %d", tr.Compiled())
	}
	if tr.Percent(false) != 98 {
		t.Fatalf("percent %d", tr.Percent(false))
	}
}
