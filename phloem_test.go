package phloem

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const t0 = int64(1_700_000_000_000_000_000)

func golden(t *testing.T, name string) []byte {
	b, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The Go binding reproduces the Rust corpus byte for byte through the C ABI
// for the frames the ABI can express (f64 uniform, explicit, gap, hello).
func TestBackdatedGolden(t *testing.T) {
	s := NewMemory()
	defer s.Close()
	if err := s.Hello("golden", "rust/golden"); err != nil {
		t.Fatal(err)
	}
	basis := Backdated(1_700_000_100_000_000)
	if err := s.Uniform(1_600_000_000_000_000_000, 20_000_000, 1, basis, []Column{{Path: "/replay/x", Values: []float64{0, 0.5, 1}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Explicit(basis, "/replay/tag", 1_600_000_000_020_000_000, 2.5); err != nil {
		t.Fatal(err)
	}
	got, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, golden(t, "backdated.phlm")) {
		t.Fatalf("backdated drifted from the golden (%d vs %d bytes)", len(got), len(golden(t, "backdated.phlm")))
	}
}

// The ended golden, every frame through the binding: hello, a uniform frame,
// the end. Control: without End, it is not the golden.
func TestEndedGolden(t *testing.T) {
	write := func(end bool) []byte {
		s := NewMemory()
		defer s.Close()
		if err := s.Hello("golden", "rust/golden"); err != nil {
			t.Fatal(err)
		}
		if err := s.Uniform(t0, 10_000_000, 1, Source, []Column{{Path: "/rig/flow", Values: []float64{1, 1.5, 2, 2.5}}}); err != nil {
			t.Fatal(err)
		}
		if end {
			if err := s.End(); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !bytes.Equal(write(true), golden(t, "ended.phlm")) {
		t.Fatalf("ended drifted from the golden")
	}
	if bytes.Equal(write(false), golden(t, "ended.phlm")) {
		t.Fatalf("control: a stream with no end matched the golden")
	}
}

// The hello_clock golden, every frame through the binding: a hello naming
// its clock, a uniform frame, the end. Controls: without WithClock it is the
// ended golden (no clock field), and an empty id is refused, nothing written.
func TestHelloClockGolden(t *testing.T) {
	write := func(opts ...HelloOption) []byte {
		s := NewMemory()
		defer s.Close()
		if err := s.Hello("golden", "rust/golden", opts...); err != nil {
			t.Fatal(err)
		}
		if err := s.Uniform(t0, 10_000_000, 1, Source, []Column{{Path: "/rig/flow", Values: []float64{1, 1.5, 2, 2.5}}}); err != nil {
			t.Fatal(err)
		}
		if err := s.End(); err != nil {
			t.Fatal(err)
		}
		got, err := s.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !bytes.Equal(write(WithClock("usb:0x0683-SN-01A2B3C4")), golden(t, "hello_clock.phlm")) {
		t.Fatalf("hello_clock drifted from the golden")
	}
	if !bytes.Equal(write(), golden(t, "ended.phlm")) {
		t.Fatalf("control: a hello with no clock is not the ended golden")
	}
	s := NewMemory()
	defer s.Close()
	if err := s.Hello("golden", "rust/golden", WithClock("")); err == nil {
		t.Fatal("an empty clock id was accepted")
	}
	if got, _ := s.Bytes(); len(got) != 0 {
		t.Fatal("a refused hello wrote bytes")
	}
	if _, err := Connect("/nonexistent/phloem/ingest.sock", "t", WithClock("")); err == nil || !strings.Contains(err.Error(), "clock id") {
		t.Fatalf("Connect with an empty clock id: %v", err)
	}
}

// The uniform_i16 / uniform_i32 goldens, every frame through the binding:
// hello, a frame of codes with one map per channel, the end. Controls:
// another map is not the golden, and a map that is not one is refused with
// nothing written.
func TestIntegerCodeGoldens(t *testing.T) {
	writeI16 := func(offset1 float64) []byte {
		s := NewMemory()
		defer s.Close()
		if err := s.Hello("golden", "rust/golden"); err != nil {
			t.Fatal(err)
		}
		err := s.UniformI16(t0, 100_000, 1, Source, []CodeColumn[int16]{
			{Path: "/adc/ai0", Codes: []int16{-32768, -1, 0, 1, 32767}, Scale: 10.0 / 32768, Offset: 0},
			{Path: "/adc/ai1", Codes: []int16{100, 200, 300, 400, 500}, Scale: 0.001, Offset: offset1,
				Valid: []bool{true, true, false, true, true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.End(); err != nil {
			t.Fatal(err)
		}
		got, err := s.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !bytes.Equal(writeI16(-2.5), golden(t, "uniform_i16.phlm")) {
		t.Fatalf("uniform_i16 drifted from the golden")
	}
	if bytes.Equal(writeI16(-2.0), golden(t, "uniform_i16.phlm")) {
		t.Fatalf("control: another map matched the golden")
	}

	s := NewMemory()
	defer s.Close()
	if err := s.Hello("golden", "rust/golden"); err != nil {
		t.Fatal(err)
	}
	strain := CodeColumn[int32]{Path: "/adc/strain", Codes: []int32{math.MinInt32, -7, 0, 7, math.MaxInt32}, Scale: 1e-6, Offset: -3}
	if err := s.UniformI32(t0, 1_000_000, 1, Source, []CodeColumn[int32]{strain}); err != nil {
		t.Fatal(err)
	}
	if err := s.End(); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Bytes(); !bytes.Equal(got, golden(t, "uniform_i32.phlm")) {
		t.Fatalf("uniform_i32 drifted from the golden")
	}

	bad := NewMemory()
	defer bad.Close()
	strain.Scale = 0
	if err := bad.UniformI32(t0, 1_000_000, 1, Source, []CodeColumn[int32]{strain}); err == nil || !strings.Contains(err.Error(), "scale must be finite") {
		t.Fatalf("a zero scale: %v", err)
	}
	if got, _ := bad.Bytes(); len(got) != 0 {
		t.Fatal("a refused frame wrote bytes")
	}
}

// The gap frame of uniform_gap, alone: the tail of that golden.
func TestGapFrameMatchesTheGoldenTail(t *testing.T) {
	s := NewMemory()
	defer s.Close()
	if err := s.Gap([]string{"/rig/pressure", "/rig/valve_open"}, t0+8_000_000, 12, "buffer overrun"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	g := golden(t, "uniform_gap.phlm")
	if !bytes.HasSuffix(g, got) {
		t.Fatalf("the gap frame is not the golden's tail")
	}
}

func TestMissingReadingIsANullNotAShift(t *testing.T) {
	s := NewMemory()
	defer s.Close()
	err := s.Uniform(t0, 100_000_000, 1, Host, []Column{
		{Path: "/t/temp", Values: []float64{21.5, 21.6, 0, 21.8}, Valid: []bool{true, true, false, true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Bytes()
	if len(got) == 0 {
		t.Fatal("nothing written")
	}
}

func TestABadBasisIsAnError(t *testing.T) {
	s := NewMemory()
	defer s.Close()
	err := s.Uniform(t0, 1, 1, TimeBasis("device"), []Column{{Path: "/a", Values: []float64{1}}})
	if err == nil {
		t.Fatal("expected an error")
	}
	got, _ := s.Bytes()
	if len(got) != 0 {
		t.Fatal("a refused frame wrote bytes")
	}
	if DefaultSocketPath() == "" {
		t.Fatal("no default path")
	}
}

// The default door is the daemon's: a socket file in the state dir on Unix,
// the daemon's named pipe on Windows (derived in the Rust core, model::door).
// Control: a PHLOEM_STATE_DIR of its own is another door.
func TestTheDefaultDoorIsThePlatformsOwn(t *testing.T) {
	t.Setenv("PHLOEM_STATE_DIR", t.TempDir())
	door := DefaultSocketPath()
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(door, `\\.\pipe\phloemd-ingest-`) {
			t.Fatalf("not the daemon's pipe: %q", door)
		}
	} else if filepath.Base(door) != "ingest.sock" {
		t.Fatalf("not the state dir's socket: %q", door)
	}
	t.Setenv("PHLOEM_STATE_DIR", t.TempDir())
	if DefaultSocketPath() == door {
		t.Fatal("another state dir named the same door")
	}
}

// Open with no key goes to the daemon on this machine: with no daemon there
// the error names the door, and a mismatched LogMany is refused before
// anything is sent. (Go is not in CI's default job; `go test ./...` here.)
func TestOpenFollowsTheKey(t *testing.T) {
	t.Setenv("PHLOEM_API_KEY", "")
	if _, err := Open("bench", WithSocketPath("/nonexistent/phloem-door.sock")); err == nil {
		t.Fatal("a door that is not there opened")
	}
	m := NewMemory()
	defer m.Close()
	if m.Route() != RouteMemory {
		t.Fatalf("a memory sink's route is %d", m.Route())
	}
	if err := m.LogMany("/x", Source, []int64{1, 2}, []float64{1}); err == nil {
		t.Fatal("2 instants for 1 value was accepted")
	}
	if err := m.LogMany("/x", Source, []int64{1700000000000000000}, []float64{1.5}); err != nil {
		t.Fatal(err)
	}
	if err := m.LogNow("/x", 1); err == nil {
		t.Fatal("a memory sink has no clock: LogNow must be refused")
	}
	if err := m.Finish(); err != nil {
		t.Fatal(err)
	}
}

// A key selects the direct route, and the library this module links HAS that
// route: against a workspace URL nothing answers at, the failure is about the
// network, never "this build has no direct route".
func TestAKeyGoesDirect(t *testing.T) {
	_, err := Open("ci", WithAPIKey("phla_test"), WithURL("http://127.0.0.1:9"))
	if err == nil {
		t.Fatal("a URL nothing listens at answered")
	}
	if strings.Contains(err.Error(), "no direct route") {
		t.Fatalf("the linked library was built without the direct route: %v", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1:9") {
		t.Fatalf("the error does not name the URL it asked: %v", err)
	}
}

// A key says nothing about where its workspace runs: without a URL it is
// refused before anything is opened, naming the variable to set.
func TestAKeyWithNoURLIsRefused(t *testing.T) {
	t.Setenv("PHLOEM_URL", "")
	_, err := Open("ci", WithAPIKey("phla_test"))
	if err == nil {
		t.Fatal("a key with no URL connected")
	}
	if !strings.Contains(err.Error(), "PHLOEM_URL") {
		t.Fatalf("the error does not say what to set: %v", err)
	}
}
