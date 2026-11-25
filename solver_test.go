package timelineconstraint

import (
	"errors"
	"math/big"
	"testing"
)

func tc(hour, minute, second, frame int64, drop bool) Timecode {
	return Timecode{
		Hour:   hour,
		Minute: minute,
		Second: second,
		Frame:  frame,
		Drop:   drop,
	}
}

func rat(numerator, denominator int64) *big.Rat {
	return big.NewRat(numerator, denominator)
}

func assertRat(t *testing.T, name string, got, want *big.Rat) {
	t.Helper()
	if got.Cmp(want) != 0 {
		t.Fatalf("%s = %s, want %s", name, got.RatString(), want.RatString())
	}
}

func TestSolve23976(t *testing.T) {
	rate := NewRate(24000, 1001, false)
	result, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 0, 0, false),
			Out: tc(0, 0, 10, 0, false),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(1, 0, 0, 0, false),
			Out:  tc(1, 0, 10, 0, false),
		},
		Speed: rat(1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.SourceDurationFrame != 240 {
		t.Fatalf("source frames = %d, want 240", result.SourceDurationFrame)
	}
	assertRat(t, "actual rate", result.ActualRate, rat(24000, 1001))
	assertRat(t, "record duration", result.RecordDuration, rat(1001, 100))
	assertRat(t, "source media duration", result.SourceMediaDuration, rat(1001, 100))
	assertRat(t, "audio start offset", result.AudioStartOffset, rat(0, 1))
	if len(result.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
}

func TestSolve2997DropFrame(t *testing.T) {
	rate := NewRate(30000, 1001, true)
	result, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 0, 0, true),
			Out: tc(0, 0, 10, 0, true),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 1, 0, 2, true),
			Out:  tc(0, 1, 10, 2, true),
		},
		Speed: rat(1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.SourceDurationFrame != 300 {
		t.Fatalf("source frames = %d, want 300", result.SourceDurationFrame)
	}
	if result.RecordInFrame != 1800 || result.RecordDurationFrame != 300 {
		t.Fatalf("record frames = in %d duration %d, want in 1800 duration 300", result.RecordInFrame, result.RecordDurationFrame)
	}
	assertRat(t, "actual rate", result.ActualRate, rat(30000, 1001))
	assertRat(t, "source media duration", result.SourceMediaDuration, rat(1001, 100))
	assertRat(t, "audio start offset", result.AudioStartOffset, rat(0, 1))
	if len(result.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
}

func TestRejectsSkippedDropFrameLabel(t *testing.T) {
	rate := NewRate(30000, 1001, true)
	_, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 1, 0, 0, true),
			Out: tc(0, 1, 10, 0, true),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 0, 0, 0, true),
			Out:  tc(0, 0, 10, 0, true),
		},
		Speed: rat(1, 1),
	})

	var conflictErr ConflictError
	if !errors.As(err, &conflictErr) || conflictErr.Conflicts[0].Code != ConflictInvalidTimecode {
		t.Fatalf("error = %#v, want invalid timecode conflict", err)
	}
}

func TestRejectsReversedRange(t *testing.T) {
	rate := NewRate(24, 1, false)
	_, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 10, 0, false),
			Out: tc(0, 0, 0, 0, false),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 10, 0, false),
		},
		Speed: rat(1, 1),
	})

	var conflictErr ConflictError
	if !errors.As(err, &conflictErr) || conflictErr.Conflicts[0].Code != ConflictInvalidRange {
		t.Fatalf("error = %#v, want invalid range conflict", err)
	}
}

func TestRejectsNonPositiveSpeed(t *testing.T) {
	rate := NewRate(24, 1, false)
	_, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 0, 0, false),
			Out: tc(0, 0, 10, 0, false),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 10, 0, false),
		},
		Speed: rat(0, 1),
	})

	var conflictErr ConflictError
	if !errors.As(err, &conflictErr) || conflictErr.Conflicts[0].Code != ConflictInvalidSpeed {
		t.Fatalf("error = %#v, want invalid speed conflict", err)
	}
}

func TestRejectsInvalidDropFrameRate(t *testing.T) {
	rate := NewRate(24, 1, true)
	_, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 0, 0, true),
			Out: tc(0, 0, 10, 0, true),
		},
		Record: RatedRange{
			Rate: NewRate(24, 1, false),
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 10, 0, false),
		},
		Speed: rat(1, 1),
	})

	var conflictErr ConflictError
	if !errors.As(err, &conflictErr) || conflictErr.Conflicts[0].Code != ConflictInvalidRate {
		t.Fatalf("error = %#v, want invalid rate conflict", err)
	}
}

func TestSolveVariableSpeed(t *testing.T) {
	rate := NewRate(24, 1, false)
	result, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 0, 0, false),
			Out: tc(0, 0, 10, 0, false),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 20, 0, false),
		},
		Speed: rat(1, 2),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertRat(t, "actual rate", result.ActualRate, rat(24, 1))
	assertRat(t, "record duration", result.RecordDuration, rat(20, 1))
	assertRat(t, "source media duration", result.SourceMediaDuration, rat(10, 1))
	assertRat(t, "audio duration drift", result.AudioDurationDrift, rat(0, 1))
	if len(result.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %#v", result.Conflicts)
	}
}

func TestSolveTwoTimesSpeed(t *testing.T) {
	rate := NewRate(24, 1, false)
	result, err := Solve(Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(0, 0, 0, 0, false),
			Out: tc(0, 0, 10, 0, false),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 5, 0, false),
		},
		Speed: rat(2, 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertRat(t, "actual rate", result.ActualRate, rat(24, 1))
	assertRat(t, "record duration", result.RecordDuration, rat(5, 1))
	assertRat(t, "source media duration", result.SourceMediaDuration, rat(10, 1))
	assertRat(t, "audio duration drift", result.AudioDurationDrift, rat(0, 1))
}

func TestReportsTimecodeAndRecordingRateConflict(t *testing.T) {
	timecodeRate := NewRate(24, 1, false)
	recordRate := NewRate(24000, 1001, false)
	result, err := Solve(Constraint{
		TimecodeRate: timecodeRate,
		Source: Range{
			In:  tc(1, 0, 0, 0, false),
			Out: tc(1, 0, 10, 0, false),
		},
		Record: RatedRange{
			Rate: recordRate,
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 10, 0, false),
		},
		Speed: rat(1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result.HasConflict(ConflictRateMismatch) {
		t.Fatalf("conflicts = %#v, want rate mismatch", result.Conflicts)
	}
	assertRat(t, "actual rate", result.ActualRate, rat(24000, 1001))
	assertRat(t, "audio start offset", result.AudioStartOffset, rat(36, 10))
	assertRat(t, "audio duration drift", result.AudioDurationDrift, rat(1, 100))

	if err := CheckRecordingRate(result, NewRate(24, 1, false)); err == nil {
		t.Fatal("expected measured 24 fps rate to conflict")
	}
	if err := CheckRecordingRate(result, recordRate); err != nil {
		t.Fatal(err)
	}
}

func TestVariableSpeedRateConflictIsStrictlyUnsatisfiable(t *testing.T) {
	rate := NewRate(24, 1, false)
	constraint := Constraint{
		TimecodeRate: rate,
		Source: Range{
			In:  tc(1, 0, 0, 0, false),
			Out: tc(1, 0, 10, 0, false),
		},
		Record: RatedRange{
			Rate: rate,
			In:   tc(0, 0, 0, 0, false),
			Out:  tc(0, 0, 10, 0, false),
		},
		Speed: rat(1, 2),
	}

	result, err := Solve(constraint)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasConflict(ConflictRateMismatch) {
		t.Fatalf("conflicts = %#v, want rate mismatch", result.Conflicts)
	}
	assertRat(t, "actual rate", result.ActualRate, rat(48, 1))
	assertRat(t, "audio start offset", result.AudioStartOffset, rat(-1800, 1))

	_, err = SolveStrict(constraint)
	var conflictErr ConflictError
	if !errors.As(err, &conflictErr) || conflictErr.Conflicts[0].Code != ConflictRateMismatch {
		t.Fatalf("strict error = %#v, want rate mismatch conflict", err)
	}
}
