// Package phloem sends data to Phloem: to the daemon on this machine, or
// straight to the cloud with an API key. It is a cgo wrapper over the C ABI of the
// Rust core: it converts arguments and returns errors, and encodes
// nothing itself — the bytes are the Rust core's, so the golden corpus holds
// here too (phloem_test.go).
//
//	c, err := phloem.Open("bench-pc")   // PHLOEM_API_KEY set: straight to the cloud
//	if err != nil { ... }
//	c.LogNow("/rig/pressure", 3.2)
//	if err := c.Finish(); err != nil { ... } // says if anything was not delivered
//
// Build the static library first, from sdk/ (`--features direct` is what
// gives Open the route to the cloud; without it a key is refused, in words):
//
//	cargo build -p phloem-sdk-c --release --features direct
//
// then `go test ./...` / `go build` in this directory links it. On Windows
// cgo links with MinGW gcc, which needs the GNU-ABI build of the library:
//
//	cargo build -p phloem-sdk-c --release --target x86_64-pc-windows-gnu
//
// The transport is the C library's: a Unix socket, or on Windows the
// daemon's named pipe — nothing in Go opens it.
package phloem

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/lib/darwin_arm64
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/lib/darwin_amd64
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/lib/linux_amd64
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/lib/linux_arm64
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/lib/windows_amd64 -lphloem -lkernel32 -lntdll -luserenv -lws2_32 -ldbghelp
#cgo !windows LDFLAGS: -lphloem -lm -lpthread
#cgo darwin LDFLAGS: -framework CoreFoundation -framework Security -framework SystemConfiguration -framework IOKit -liconv
#cgo linux LDFLAGS: -ldl
#include <stdlib.h>
#include "phloem.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// TimeBasis says which clock a frame's stamps are on. There is no default.
type TimeBasis string

const (
	// Source: the producer's own clock (a device, a bus, a recording).
	Source TimeBasis = "source"
	// Host: this machine's wall clock, read as the producer sampled.
	Host TimeBasis = "host"
)

// Backdated is a replayed recording: stamps from an earlier run, with the
// wall-clock instant (µs since the epoch) the replay began.
func Backdated(anchorUS int64) TimeBasis {
	return TimeBasis(fmt.Sprintf("backdated:%d", anchorUS))
}

// Channel is one declaration: a path, and optionally what to draw it as
// and its unit.
type Channel struct {
	Path      string
	Archetype string
	Unit      string
}

// Column is one channel's samples in a uniform frame. A nil Valid means
// every reading is present; otherwise false marks a missing one (a NULL on
// the wire, counted by the daemon, never shifted).
type Column struct {
	Path   string
	Values []float64
	Valid  []bool
}

// Sink is where a program's data goes: the daemon on this machine, the
// cloud, or an in-memory writer. Open makes one.
type Sink struct {
	p *C.phloem_sink
}

func lastError() error {
	buf := make([]byte, 512)
	n := C.phloem_last_error((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if int(n) > len(buf)-1 {
		n = C.size_t(len(buf) - 1)
	}
	return errors.New(string(buf[:n]))
}

func check(rc C.int) error {
	if rc != 0 {
		return lastError()
	}
	return nil
}

// HelloOption adds an optional statement to the hello.
type HelloOption func(*hello)

type hello struct {
	clockID *string
}

// WithClock names the clock the producer's stamps come off — a device serial,
// a bus id: something that names the same clock across reconnects. The daemon
// keeps what it learned of a named clock's drift when the program
// reconnects; an unnamed one is re-learned from scratch on every connection.
// An empty id is refused before anything is written.
func WithClock(id string) HelloOption {
	return func(h *hello) { h.clockID = &id }
}

func helloOptions(opts []HelloOption) (*C.char, func()) {
	var h hello
	for _, o := range opts {
		o(&h)
	}
	if h.clockID == nil {
		return nil, func() {}
	}
	c := C.CString(*h.clockID)
	return c, func() { C.free(unsafe.Pointer(c)) }
}

// Connect opens the daemon's SDK door (path "" = the daemon's own default
// location) and says hello as producer, with WithClock to name its clock.
// The daemon refuses a producer that has no enabled SDK input; the error
// says so, and the attempt appears in Inputs as a discovered candidate.
func Connect(path, producer string, opts ...HelloOption) (*Sink, error) {
	cp := C.CString(producer)
	defer C.free(unsafe.Pointer(cp))
	var cpath *C.char
	if path != "" {
		cpath = C.CString(path)
		defer C.free(unsafe.Pointer(cpath))
	}
	cclock, free := helloOptions(opts)
	defer free()
	p := C.phloem_connect_with_clock(cpath, cp, cclock)
	if p == nil {
		return nil, lastError()
	}
	return &Sink{p: p}, nil
}

// OpenOption sets one thing about how Open connects.
type OpenOption func(*openOptions)

type openOptions struct {
	apiKey, url, socketPath, clockID string
}

// WithAPIKey sends straight to the cloud with this key. Without it Open reads
// PHLOEM_API_KEY, and with neither it uses the Phloem on this machine. A key
// needs its workspace's URL beside it (WithURL, or PHLOEM_URL).
func WithAPIKey(key string) OpenOption { return func(o *openOptions) { o.apiKey = key } }

// WithURL names the workspace's URL (the direct route): the URL shown beside
// the API key on the account page. Without it Open reads PHLOEM_URL; a key
// with neither is refused.
func WithURL(url string) OpenOption { return func(o *openOptions) { o.url = url } }

// WithSocketPath names the daemon's door (the local route). Default: the
// daemon's own location.
func WithSocketPath(path string) OpenOption { return func(o *openOptions) { o.socketPath = path } }

// WithClockID names the clock the stamps come off (the local route) — see
// WithClock.
func WithClockID(id string) OpenOption { return func(o *openOptions) { o.clockID = id } }

// Route is which way a sink's data goes.
type Route int

const (
	// RouteLocal is the Phloem daemon on this machine.
	RouteLocal Route = 0
	// RouteDirect is straight to the cloud.
	RouteDirect Route = 1
	// RouteMemory is an in-memory writer (NewMemory).
	RouteMemory Route = 2
)

// Open is the one call a program needs. With an API key (WithAPIKey, or
// PHLOEM_API_KEY) the data goes straight to the cloud as the program name;
// with none it goes to the Phloem daemon on this machine as the producer
// name. A key needs its workspace's URL (WithURL, or PHLOEM_URL). Route says
// which way it went; end with Finish.
func Open(name string, opts ...OpenOption) (*Sink, error) {
	var o openOptions
	for _, f := range opts {
		f(&o)
	}
	cs := func(v string) *C.char {
		if v == "" {
			return nil
		}
		return C.CString(v)
	}
	cname := C.CString(name)
	ckey, curl, cpath, cclock := cs(o.apiKey), cs(o.url), cs(o.socketPath), cs(o.clockID)
	defer func() {
		for _, p := range []*C.char{cname, ckey, curl, cpath, cclock} {
			if p != nil {
				C.free(unsafe.Pointer(p))
			}
		}
	}()
	p := C.phloem_open(cname, ckey, curl, cpath, cclock)
	if p == nil {
		return nil, lastError()
	}
	return &Sink{p: p}, nil
}

// Route is which way this sink's data goes.
func (s *Sink) Route() Route { return Route(C.phloem_route(s.p)) }

// LogNow writes one reading, stamped with this machine's clock as the call
// is made.
func (s *Sink) LogNow(path string, value float64) error {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	return check(C.phloem_log_now(s.p, cp, C.double(value)))
}

// LogMany writes many readings of one channel, each at its own instant:
// timesNS[i] on the stated basis with values[i].
func (s *Sink) LogMany(path string, basis TimeBasis, timesNS []int64, values []float64) error {
	if len(timesNS) != len(values) {
		return fmt.Errorf("%s: %d instants for %d values", path, len(timesNS), len(values))
	}
	if len(values) == 0 {
		return nil
	}
	cp, cb := C.CString(path), C.CString(string(basis))
	defer C.free(unsafe.Pointer(cp))
	defer C.free(unsafe.Pointer(cb))
	return check(C.phloem_log_many(s.p, cp, cb,
		(*C.int64_t)(unsafe.Pointer(&timesNS[0])), (*C.double)(unsafe.Pointer(&values[0])),
		C.size_t(len(values))))
}

// Finish ENDS the stream, flushes, releases the sink and says how it went.
// On the direct route it waits for the cloud to confirm what was sent and
// returns an error when anything was not — what a CI job turns into its exit
// code. Close is Finish with the answer thrown away.
func (s *Sink) Finish() error {
	if s.p == nil {
		return nil
	}
	p := s.p
	s.p = nil
	return check(C.phloem_finish(p))
}

// NewMemory is a sink that keeps its frames; Bytes returns them.
func NewMemory() *Sink {
	return &Sink{p: C.phloem_memory_new()}
}

// Hello writes the opening frame of a memory sink (a connected sink sent its
// own); WithClock names the producer's clock.
func (s *Sink) Hello(producer, sdk string, opts ...HelloOption) error {
	cp, cs := C.CString(producer), C.CString(sdk)
	defer C.free(unsafe.Pointer(cp))
	defer C.free(unsafe.Pointer(cs))
	cclock, free := helloOptions(opts)
	defer free()
	return check(C.phloem_hello_with_clock(s.p, cp, cs, cclock))
}

// Bytes is everything a memory sink has written so far.
func (s *Sink) Bytes() ([]byte, error) {
	var n C.size_t
	b := C.phloem_memory_bytes(s.p, &n)
	if b == nil {
		return nil, lastError()
	}
	return C.GoBytes(unsafe.Pointer(b), C.int(n)), nil
}

func cStrings(ss []string) ([]*C.char, func()) {
	out := make([]*C.char, len(ss))
	for i, s := range ss {
		if s == "" {
			out[i] = nil
		} else {
			out[i] = C.CString(s)
		}
	}
	return out, func() {
		for _, p := range out {
			if p != nil {
				C.free(unsafe.Pointer(p))
			}
		}
	}
}

func ptrOf(cs []*C.char) **C.char {
	if len(cs) == 0 {
		return nil
	}
	return (**C.char)(unsafe.Pointer(&cs[0]))
}

// Declare channels. Idempotent; a path may be declared again with more.
func (s *Sink) Declare(channels []Channel) error {
	if len(channels) == 0 {
		return nil
	}
	paths := make([]string, len(channels))
	arche := make([]string, len(channels))
	units := make([]string, len(channels))
	for i, c := range channels {
		paths[i], arche[i], units[i] = c.Path, c.Archetype, c.Unit
	}
	cp, fp := cStrings(paths)
	defer fp()
	ca, fa := cStrings(arche)
	defer fa()
	cu, fu := cStrings(units)
	defer fu()
	return check(C.phloem_declare(s.p, ptrOf(cp), ptrOf(ca), ptrOf(cu), C.size_t(len(channels))))
}

// Uniform writes a frame whose columns are sampled at t0 + k*step, step
// being stepNumNS/stepDen nanoseconds. Columns must be the same length.
func (s *Sink) Uniform(t0NS int64, stepNumNS, stepDen uint64, basis TimeBasis, cols []Column) error {
	if len(cols) == 0 {
		return errors.New("a uniform frame needs at least one channel")
	}
	rows := len(cols[0].Values)
	values := make([]float64, 0, rows*len(cols))
	var valid []byte
	anyValid := false
	for _, c := range cols {
		if c.Valid != nil {
			anyValid = true
		}
	}
	paths := make([]string, len(cols))
	for i, c := range cols {
		if len(c.Values) != rows {
			return fmt.Errorf("%s has %d samples, %s has %d — a uniform frame's channels are sampled at the same instants", c.Path, len(c.Values), cols[0].Path, rows)
		}
		if c.Valid != nil && len(c.Valid) != rows {
			return fmt.Errorf("%s has %d validity flags for %d samples", c.Path, len(c.Valid), rows)
		}
		paths[i] = c.Path
		values = append(values, c.Values...)
		if anyValid {
			for k := 0; k < rows; k++ {
				v := byte(1)
				if c.Valid != nil && !c.Valid[k] {
					v = 0
				}
				valid = append(valid, v)
			}
		}
	}
	cp, fp := cStrings(paths)
	defer fp()
	cb := C.CString(string(basis))
	defer C.free(unsafe.Pointer(cb))
	var vptr *C.double
	if rows > 0 {
		vptr = (*C.double)(unsafe.Pointer(&values[0]))
	}
	var validPtr *C.uint8_t
	if anyValid && len(valid) > 0 {
		validPtr = (*C.uint8_t)(unsafe.Pointer(&valid[0]))
	}
	return check(C.phloem_uniform_f64(s.p, C.int64_t(t0NS), C.uint64_t(stepNumNS), C.uint64_t(stepDen), cb,
		ptrOf(cp), C.size_t(len(cols)), vptr, C.size_t(rows), validPtr))
}

// CodeColumn is one channel's converter CODES in a uniform frame, with the
// exact map to their value: code*Scale + Offset. The codes cross the wire as
// 2 (int16) or 4 (int32) bytes a sample and the map beside them, so the value
// stays exact. Scale must be finite and non-zero, Offset finite; the core
// refuses anything else with nothing written. Valid as in Column. Codes are
// uniform-only: an explicit sample is a value.
type CodeColumn[T int16 | int32] struct {
	Path   string
	Codes  []T
	Scale  float64
	Offset float64
	Valid  []bool
}

func flattenCodes[T int16 | int32](cols []CodeColumn[T]) (flat []T, paths []string, scales, offsets []float64, valid []byte, rows int, err error) {
	if len(cols) == 0 {
		return nil, nil, nil, nil, nil, 0, errors.New("a uniform frame needs at least one channel")
	}
	rows = len(cols[0].Codes)
	anyValid := false
	for _, c := range cols {
		if c.Valid != nil {
			anyValid = true
		}
	}
	for _, c := range cols {
		if len(c.Codes) != rows {
			return nil, nil, nil, nil, nil, 0, fmt.Errorf("%s has %d samples, %s has %d — a uniform frame's channels are sampled at the same instants", c.Path, len(c.Codes), cols[0].Path, rows)
		}
		if c.Valid != nil && len(c.Valid) != rows {
			return nil, nil, nil, nil, nil, 0, fmt.Errorf("%s has %d validity flags for %d samples", c.Path, len(c.Valid), rows)
		}
		paths = append(paths, c.Path)
		scales = append(scales, c.Scale)
		offsets = append(offsets, c.Offset)
		flat = append(flat, c.Codes...)
		if anyValid {
			for k := 0; k < rows; k++ {
				v := byte(1)
				if c.Valid != nil && !c.Valid[k] {
					v = 0
				}
				valid = append(valid, v)
			}
		}
	}
	return flat, paths, scales, offsets, valid, rows, nil
}

// UniformI16 writes a frame of 16-bit converter codes, each column with its
// own map, sampled at t0 + k*step (step = stepNumNS/stepDen ns).
func (s *Sink) UniformI16(t0NS int64, stepNumNS, stepDen uint64, basis TimeBasis, cols []CodeColumn[int16]) error {
	flat, paths, scales, offsets, valid, rows, err := flattenCodes(cols)
	if err != nil {
		return err
	}
	var cptr *C.int16_t
	if len(flat) > 0 {
		cptr = (*C.int16_t)(unsafe.Pointer(&flat[0]))
	}
	return s.uniformCodes(basis, paths, scales, offsets, valid,
		func(cb *C.char, cp **C.char, sp, op *C.double, vp *C.uint8_t) C.int {
			return C.phloem_uniform_i16(s.p, C.int64_t(t0NS), C.uint64_t(stepNumNS), C.uint64_t(stepDen), cb,
				cp, C.size_t(len(paths)), cptr, sp, op, C.size_t(rows), vp)
		})
}

// UniformI32 writes a frame of 32-bit converter codes, as UniformI16.
func (s *Sink) UniformI32(t0NS int64, stepNumNS, stepDen uint64, basis TimeBasis, cols []CodeColumn[int32]) error {
	flat, paths, scales, offsets, valid, rows, err := flattenCodes(cols)
	if err != nil {
		return err
	}
	var cptr *C.int32_t
	if len(flat) > 0 {
		cptr = (*C.int32_t)(unsafe.Pointer(&flat[0]))
	}
	return s.uniformCodes(basis, paths, scales, offsets, valid,
		func(cb *C.char, cp **C.char, sp, op *C.double, vp *C.uint8_t) C.int {
			return C.phloem_uniform_i32(s.p, C.int64_t(t0NS), C.uint64_t(stepNumNS), C.uint64_t(stepDen), cb,
				cp, C.size_t(len(paths)), cptr, sp, op, C.size_t(rows), vp)
		})
}

// uniformCodes holds the C strings and pointers both code widths share for
// the one call that writes the frame.
func (s *Sink) uniformCodes(basis TimeBasis, paths []string, scales, offsets []float64, valid []byte,
	call func(cb *C.char, cp **C.char, sp, op *C.double, vp *C.uint8_t) C.int) error {
	cp, fp := cStrings(paths)
	defer fp()
	cb := C.CString(string(basis))
	defer C.free(unsafe.Pointer(cb))
	var validPtr *C.uint8_t
	if len(valid) > 0 {
		validPtr = (*C.uint8_t)(unsafe.Pointer(&valid[0]))
	}
	return check(call(cb, ptrOf(cp), (*C.double)(unsafe.Pointer(&scales[0])), (*C.double)(unsafe.Pointer(&offsets[0])), validPtr))
}

// Unstamped marks an explicit sample the daemon stamps on arrival.
const Unstamped int64 = -1

// Explicit writes one sample with its own instant (Unstamped = on arrival).
// value is float64, int64, bool or string.
func (s *Sink) Explicit(basis TimeBasis, path string, tNS int64, value any) error {
	cb, cpath := C.CString(string(basis)), C.CString(path)
	defer C.free(unsafe.Pointer(cb))
	defer C.free(unsafe.Pointer(cpath))
	var kind C.char
	var f C.double
	var i C.int64_t
	var text *C.char
	switch v := value.(type) {
	case float64:
		kind, f = 'f', C.double(v)
	case float32:
		kind, f = 'f', C.double(v)
	case int:
		kind, i = 'i', C.int64_t(v)
	case int64:
		kind, i = 'i', C.int64_t(v)
	case bool:
		kind = 'b'
		if v {
			i = 1
		}
	case string:
		kind = 't'
		text = C.CString(v)
		defer C.free(unsafe.Pointer(text))
	default:
		return fmt.Errorf("%s: a value is a float64, int64, bool or string, not %T", path, value)
	}
	return check(C.phloem_explicit(s.p, cb, cpath, C.int64_t(tNS), kind, f, i, text))
}

// Gap states a hole on paths whose good data stopped at frontierNS;
// atLeastSamples < 0 means the width is unknown.
func (s *Sink) Gap(paths []string, frontierNS int64, atLeastSamples int64, detail string) error {
	cp, fp := cStrings(paths)
	defer fp()
	cd := C.CString(detail)
	defer C.free(unsafe.Pointer(cd))
	return check(C.phloem_gap(s.p, ptrOf(cp), C.size_t(len(paths)), C.int64_t(frontierNS), C.int64_t(atLeastSamples), cd))
}

// Flush pushes buffered frames to the daemon.
func (s *Sink) Flush() error { return check(C.phloem_flush(s.p)) }

// End writes the END frame into a memory sink: the last frame of a stream
// that ends by its own word. A connection is ended by Close, which sends it.
func (s *Sink) End() error { return check(C.phloem_end(s.p)) }

// Close ENDS the stream, flushes and releases the sink. A connection never
// closed — the program killed or crashed — is read by the daemon as a cut, and
// the outage of its channels is stated (producer_cut). Close it with defer.
func (s *Sink) Close() {
	if s.p != nil {
		C.phloem_close(s.p)
		s.p = nil
	}
}

// DefaultSocketPath is where the local daemon's SDK door is, by the
// daemon's own rule.
func DefaultSocketPath() string {
	buf := make([]byte, 1024)
	n := C.phloem_default_socket_path((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if int(n) > len(buf)-1 {
		n = C.size_t(len(buf) - 1)
	}
	return string(buf[:n])
}
