package panel

import (
	"os"
	"sort"
	"testing"
)

func TestParseProcStat(t *testing.T) {
	comm, ut, st, err := parseProcStat("1234 (wdtt-app) S 1 2 3 4 5 6 7 0 9 10 100 200 300 400 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30 31 32 33 34 35 36 37 38 39 40 41 42 43 44")
	if err != nil {
		t.Fatal(err)
	}
	if comm != "wdtt-app" || ut != 100 || st != 200 {
		t.Fatalf("unexpected parse: comm=%q ut=%d st=%d", comm, ut, st)
	}
}

func TestParseProcStatNameWithSpacesAndParens(t *testing.T) {
	// comm field can itself contain spaces and parentheses
	comm, ut, st, err := parseProcStat("42 (Web Content (tab)) R 1 2 3 4 5 6 7 8 9 10 55 66 13 14 15")
	if err != nil {
		t.Fatal(err)
	}
	if comm != "Web Content (tab)" || ut != 55 || st != 66 {
		t.Fatalf("unexpected parse: comm=%q ut=%d st=%d", comm, ut, st)
	}
}

func TestParseProcStatErrors(t *testing.T) {
	if _, _, _, err := parseProcStat("garbage-without-parens"); err == nil {
		t.Fatal("expected error for missing parens")
	}
	if _, _, _, err := parseProcStat("1 (short) R 1 2 3"); err == nil {
		t.Fatal("expected error for short stat line")
	}
}

func TestIsProcessKillable(t *testing.T) {
	if isProcessKillable(1) {
		t.Fatal("pid 1 must not be killable")
	}
	if isProcessKillable(0) {
		t.Fatal("pid 0 must not be killable")
	}
	if isProcessKillable(-5) {
		t.Fatal("negative pid must not be killable")
	}
	if isProcessKillable(os.Getpid()) {
		t.Fatal("self must not be killable")
	}
	if !isProcessKillable(99999) {
		t.Fatal("regular pid should be killable")
	}
}

func TestKillProcessGuards(t *testing.T) {
	if err := killProcess(1); err == nil {
		t.Fatal("killing pid 1 must be rejected")
	}
	if err := killProcess(os.Getpid()); err == nil {
		t.Fatal("killing self must be rejected")
	}
}

func TestRoundProcessCPU(t *testing.T) {
	cases := map[float64]float64{
		12.345: 12.35,
		0.004:  0.0,
		99.999: 100.0,
		6.961:  6.96,
	}
	for in, want := range cases {
		if got := roundProcessCPU(in); got != want {
			t.Fatalf("roundProcessCPU(%v)=%v want %v", in, got, want)
		}
	}
}

func TestGetProcessListSortedByPID(t *testing.T) {
	list := getProcessList(40)
	if len(list) == 0 {
		t.Skip("no /proc entries available in this environment")
	}
	if len(list) > 40 {
		t.Fatalf("limit not respected: got %d", len(list))
	}
	pids := make([]int, len(list))
	for i, p := range list {
		pids[i] = p.PID
		if p.Name == "" {
			t.Fatalf("process %d has empty name", p.PID)
		}
		if p.Mem == 0 {
			t.Fatalf("process %d has zero mem (should be filtered)", p.PID)
		}
	}
	if !sort.IntsAreSorted(pids) {
		t.Fatalf("process list is not sorted by PID: %v", pids)
	}
}

func TestGetProcessListSelfNotKillable(t *testing.T) {
	// our own pid should appear (it has RSS) and be marked non-killable
	list := getProcessList(4096)
	self := os.Getpid()
	for _, p := range list {
		if p.PID == self && p.Killable {
			t.Fatal("self process must be marked non-killable")
		}
		if p.PID == 1 && p.Killable {
			t.Fatal("pid 1 must be marked non-killable")
		}
	}
}
