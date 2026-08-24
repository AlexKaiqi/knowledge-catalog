package main

import "testing"

func TestClassifyApplyReplayAndRegression(t *testing.T) {
	event := binlogEvent{BinlogFile: "mysql-bin.000003", Position: 687, EventID: "evt-687"}
	if got, err := classify(event, "digest-a", checkpoint{}, false); err != nil || got != "APPLY" {
		t.Fatalf("new event = %q, %v", got, err)
	}
	prior := checkpoint{BinlogFile: "mysql-bin.000003", Position: 687, EventID: "evt-687", PayloadDigest: "digest-a"}
	if got, err := classify(event, "digest-a", prior, true); err != nil || got != "REPLAY" {
		t.Fatalf("replay = %q, %v", got, err)
	}
	event.Position = 686
	if _, err := classify(event, "digest-a", prior, true); err == nil {
		t.Fatal("position regression accepted")
	}
}

func TestClassifyRejectsConflictAtSamePosition(t *testing.T) {
	prior := checkpoint{BinlogFile: "mysql-bin.000003", Position: 687, EventID: "evt-687", PayloadDigest: "digest-a"}
	event := binlogEvent{BinlogFile: "mysql-bin.000003", Position: 687, EventID: "evt-other"}
	if _, err := classify(event, "digest-b", prior, true); err == nil {
		t.Fatal("same position conflict accepted")
	}
}

func TestComparePositionAcrossFiles(t *testing.T) {
	got, err := comparePosition("mysql-bin.000004", 4, "mysql-bin.000003", 9999)
	if err != nil || got != 1 {
		t.Fatalf("compare = %d, %v", got, err)
	}
	if _, _, err := splitBinlogFile("mysql-bin"); err == nil {
		t.Fatal("invalid binlog name accepted")
	}
}
