/* phloem.h — send data to Phloem from C or C++.
 *
 * Generated from the phloem-sdk Rust core's C ABI; do not edit.
 *
 * Every call returns 0 on success and -1 on failure; the failure's words are
 * read with phloem_last_error(). Strings are NUL-terminated UTF-8.
 *
 *   phloem_sink *s = phloem_open("bench-pc", NULL, NULL, NULL, NULL);
 *   if (!s) { char why[256]; phloem_last_error(why, sizeof why); ... }
 *   phloem_log_now(s, "/rig/pressure", 3.2);
 *   const char *paths[] = {"/rig/pressure"};
 *   phloem_uniform_f64(s, t0_ns, 1000000, 1, "host", paths, 1, values, rows, NULL);
 *   return phloem_finish(s) ? 1 : 0;
 *
 * With PHLOEM_URL and PHLOEM_API_KEY set (or a URL and a key passed to
 * phloem_open: phloem_open("ci", key, url, NULL, NULL)) the data goes straight
 * to the cloud; with no key it goes to the Phloem daemon on this machine.
 * C++: see phloem.hpp.
 */

#ifndef PHLOEM_H
#define PHLOEM_H

#include <stddef.h>
#include <stdint.h>

// Where a program's data goes: the Phloem daemon on this machine, straight
// to the cloud, or memory (for a program that ships frames some other way,
// and for the golden test). Made by `phloem_open`, ended by `phloem_finish`.
typedef struct phloem_sink phloem_sink;

#ifdef __cplusplus
extern "C" {
#endif // __cplusplus

// The last failure's words, copied into `buf` (truncated to `len - 1`,
// always NUL-terminated). Returns the full length of the message.
//
// # Safety
// `buf` points to `len` writable bytes, or `len` is 0.
size_t phloem_last_error(char *buf, size_t len);

// Connect to the local daemon's SDK door and say hello as `producer`.
// `path` may be NULL for the daemon's own default location. NULL on
// failure (see `phloem_last_error`).
//
// # Safety
// `path` is NULL or a valid string; `producer` is a valid string.
struct phloem_sink *phloem_connect(const char *path, const char *producer);

// `phloem_connect`, stating the clock the producer's stamps come off
// (`clock_id`: a device serial, a bus id; NULL = unstated). The daemon keeps
// what it learned of a NAMED clock's drift across reconnects. An empty id is
// refused before the socket is opened.
//
// # Safety
// `path`/`clock_id` are NULL or valid strings; `producer` is a valid string.
struct phloem_sink *phloem_connect_with_clock(const char *path,
                                              const char *producer,
                                              const char *clock_id);

// An in-memory sink; its bytes are read with `phloem_memory_bytes`.
struct phloem_sink *phloem_memory_new(void);

// The hello frame, for a memory sink (a connected sink sent its own).
//
// # Safety
// `s` came from this library; `producer`/`sdk` are valid strings.
int phloem_hello(struct phloem_sink *s, const char *producer, const char *sdk);

// `phloem_hello` naming the producer's clock (`clock_id` NULL = unstated,
// the bytes of `phloem_hello`; empty = refused, nothing written).
//
// # Safety
// `s` came from this library; `producer`/`sdk` are valid strings;
// `clock_id` is NULL or a valid string.
int phloem_hello_with_clock(struct phloem_sink *s,
                            const char *producer,
                            const char *sdk,
                            const char *clock_id);

// Everything a memory sink has written so far. `*len` receives the byte
// count; the pointer is valid until the next call on this sink.
//
// # Safety
// `s` came from `phloem_memory_new`; `len` is a valid pointer.
const uint8_t *phloem_memory_bytes(struct phloem_sink *s, size_t *len);

// Declare channels: `paths[i]` with optional `archetypes[i]` / `units[i]`
// (each array may be NULL, as may any entry).
//
// # Safety
// Arrays have `n` entries of valid strings or NULL.
int phloem_declare(struct phloem_sink *s,
                   const char *const *paths,
                   const char *const *archetypes,
                   const char *const *units,
                   size_t n);

// A uniform frame of f64 channels: `values` is `n_ch * rows` doubles,
// channel-major; `valid` is NULL or `n_ch * rows` bytes (0 = missing).
//
// # Safety
// Arrays are sized as stated; strings are valid.
int phloem_uniform_f64(struct phloem_sink *s,
                       int64_t t0_ns,
                       uint64_t step_num_ns,
                       uint64_t step_den,
                       const char *time_basis,
                       const char *const *paths,
                       size_t n_ch,
                       const double *values,
                       size_t rows,
                       const uint8_t *valid);

// A uniform frame of 16-bit converter CODES (a converter's raw output): `codes` is
// `n_ch * rows` int16, channel-major, and channel `ch` reads as
// `codes[ch * rows + k] * scales[ch] + offsets[ch]` — `scales`/`offsets` hold
// one map per channel. A scale must be finite and non-zero, an offset
// finite; anything else is refused with nothing written. `valid` as in
// `phloem_uniform_f64`. Codes are uniform-only: an explicit sample is a value.
//
// # Safety
// Arrays are sized as stated; strings are valid.
int phloem_uniform_i16(struct phloem_sink *s,
                       int64_t t0_ns,
                       uint64_t step_num_ns,
                       uint64_t step_den,
                       const char *time_basis,
                       const char *const *paths,
                       size_t n_ch,
                       const int16_t *codes,
                       const double *scales,
                       const double *offsets,
                       size_t rows,
                       const uint8_t *valid);

// `phloem_uniform_i16` for 32-bit codes.
//
// # Safety
// Arrays are sized as stated; strings are valid.
int phloem_uniform_i32(struct phloem_sink *s,
                       int64_t t0_ns,
                       uint64_t step_num_ns,
                       uint64_t step_den,
                       const char *time_basis,
                       const char *const *paths,
                       size_t n_ch,
                       const int32_t *codes,
                       const double *scales,
                       const double *offsets,
                       size_t rows,
                       const uint8_t *valid);

// One explicit sample. `t_ns < 0` means unstamped (the daemon stamps it on
// arrival). The value is one of `f64` / `i64` / `bool` / text by `kind`:
// `'f'`, `'i'`, `'b'`, `'t'`.
//
// # Safety
// Strings are valid; `text` is valid when `kind == 't'`.
int phloem_explicit(struct phloem_sink *s,
                    const char *time_basis,
                    const char *path,
                    int64_t t_ns,
                    char kind,
                    double f,
                    int64_t i,
                    const char *text);

// State a hole on `paths` whose good data stopped at `frontier_ns`;
// `at_least_samples < 0` means the width is unknown.
//
// # Safety
// `paths` has `n` valid strings; `detail` is a valid string.
int phloem_gap(struct phloem_sink *s,
               const char *const *paths,
               size_t n,
               int64_t frontier_ns,
               int64_t at_least_samples,
               const char *detail);

// Push buffered frames to the daemon.
//
// # Safety
// `s` came from this library.
int phloem_flush(struct phloem_sink *s);

// Write the END frame into a memory sink — the last frame of a stream that
// ends by its own word. A connection is ended by `phloem_close`, which sends
// it; calling this on one is refused (nothing may follow the end).
//
// # Safety
// `s` came from this library.
int phloem_end(struct phloem_sink *s);

// END the stream, flush and free. A connection that is never closed — the
// program killed or crashed — is read by the daemon as a cut, and the outage of
// its channels is stated. `s` may be NULL.
//
// # Safety
// `s` is NULL or came from this library and is not used afterwards.
void phloem_close(struct phloem_sink *s);

// Open a sink the way its arguments say — the ONE call a program needs.
// With an API key (`api_key`, or `PHLOEM_API_KEY` when it is NULL) the data
// goes straight to the cloud as the program `name`; with none it goes to the
// Phloem daemon on this machine as the producer `name`. A key needs its
// workspace's URL beside it: `url` (direct; NULL reads `PHLOEM_URL`) is the
// URL shown beside the API key on the account page, and a key with no URL
// is refused. `socket_path` and `clock_id` (local) may each be NULL for
// their defaults. NULL on failure (see `phloem_last_error`). `phloem_route`
// says which way it went.
//
// A library built without the direct route refuses a key here, in words.
//
// # Safety
// `name` is a valid string; the others are NULL or valid strings.
struct phloem_sink *phloem_open(const char *name,
                                const char *api_key,
                                const char *url,
                                const char *socket_path,
                                const char *clock_id);

// Which way a sink's data goes: 0 = the daemon on this machine, 1 = straight
// to the cloud, 2 = memory. -1 for NULL.
//
// # Safety
// `s` is NULL or came from this library.
int phloem_route(const struct phloem_sink *s);

// One reading, now: `value` at `path`, stamped with this machine's clock as
// the call is made (the `host` basis). Refused on a memory sink, whose bytes
// would differ on every run.
//
// # Safety
// `s` came from this library; `path` is a valid string.
int phloem_log_now(struct phloem_sink *s, const char *path, double value);

// Many readings of one channel, each at its own instant: `times_ns[i]` on
// the stated basis with `values[i]`, `n` of them.
//
// # Safety
// `s` came from this library; strings are valid; both arrays hold `n`.
int phloem_log_many(struct phloem_sink *s,
                    const char *path,
                    const char *time_basis,
                    const int64_t *times_ns,
                    const double *values,
                    size_t n);

// END the stream, flush, free the sink, and SAY how it went: 0 when
// everything was delivered, -1 otherwise (`phloem_last_error` has the
// words). On the direct route this waits for the cloud to confirm what was
// sent and fails when anything was not — what a CI job returns as its exit
// code. `phloem_close` is this with the answer thrown away. `s` may be NULL.
//
// # Safety
// `s` is NULL or came from this library and is not used afterwards.
int phloem_finish(struct phloem_sink *s);

// The daemon's default door, by its own rule (`$PHLOEM_STATE_DIR/ingest.sock`,
// else `~/.phloem/ingest.sock`; on Windows the daemon's named pipe
// `\\.\pipe\phloemd-ingest-<id>`), copied into `buf`. Returns the full length.
//
// # Safety
// `buf` points to `len` writable bytes, or `len` is 0.
size_t phloem_default_socket_path(char *buf, size_t len);

#ifdef __cplusplus
}  // extern "C"
#endif  // __cplusplus

#endif  /* PHLOEM_H */
