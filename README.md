# Phloem SDK for Go

Send readings from Go to [Phloem](https://phloem.ai). Each reading becomes a
named channel you can watch live, replay, chart and check, the same as one
from a connector.

```go
package main

import (
	"log"

	phloem "github.com/phloem-labs/phloem-go"
)

func main() {
	s, err := phloem.Open("hil-runner")
	if err != nil {
		log.Fatal(err)
	}
	if err := s.LogNow("/rig/psi", 3.2); err != nil {
		log.Fatal(err)
	}
	if err := s.Finish(); err != nil {
		log.Fatal(err) // something was not delivered
	}
}
```

## Install

```sh
go get github.com/phloem-labs/phloem-go
```

Linux and macOS, on x64 and Arm64. Phloem's compiled core ships inside the
module, so there is nothing else to download. It links through cgo, which
needs a C compiler on the machine (`CGO_ENABLED=1`, the default when one is
present). Windows is not supported yet.

## Two ways to send

`Open` picks the way by whether it is given an API key.

| | Through Phloem on this machine | Straight to the cloud |
|---|---|---|
| **When** | no API key | an API key is set |
| **Needs** | Phloem Computer, or the Phloem service on a server; the program allowed once on **Inputs** | a workspace API key and your workspace's URL, both on the account page |
| **If the network drops** | nothing is lost: Phloem keeps the data on disk and uploads it later | data waits in memory (64 MiB by default) |
| **For** | test stands, anything with hardware | CI jobs, simulations |

To send straight to the cloud, set the two values the account page shows
beside a new key:

```sh
export PHLOEM_URL=https://api.<your-environment>.phloem.ai
export PHLOEM_API_KEY=phla_…
```

or pass them: `phloem.Open("ci", phloem.WithURL(url), phloem.WithAPIKey(key))`.
`s.Route()` says which way the data is going.

## Sending

| Method | What it does |
|---|---|
| `LogNow(path, v float64)` | one reading, stamped with this machine's clock |
| `LogMany(path, basis, timesNS []int64, values []float64)` | many readings of one channel |
| `Explicit(basis, path, tNS, value)` | one reading at its own time; `value` is `float64`, `int64`, `bool` or `string` |
| `Uniform(t0NS, stepNumNS, stepDen, basis, cols []Column)` | a block of evenly spaced samples for several channels |
| `Gap(paths, frontierNS, atLeastSamples, detail)` | state that data is missing; `atLeastSamples < 0` when the width is unknown |
| `Declare(channels []Channel)` | a channel's unit and how to draw it (through Phloem on this machine) |
| `Flush()` | send what is held now |
| `Finish()` | end the stream; returns an error if anything was not delivered |

| Option for `Open` | What it does |
|---|---|
| `WithAPIKey(key)` | send straight to the cloud |
| `WithURL(url)` | your workspace's URL, needed with a key |
| `WithClockID(id)` | name the clock your timestamps come from, such as a device serial |

**Timestamps** always say which clock they are on: `phloem.Host` (this
machine's), `phloem.Source` (your device's or recording's own) or
`phloem.Backdated(anchorUS)` (a recording replayed now). There is no default.

## Good to know

- **`Finish` tells you whether anything was lost.** Straight to the cloud, it
  waits for the cloud to confirm and returns an error with the exact count if
  anything was not stored, so a CI job fails instead of passing silently.
- **One program per name.** A second program started under the same name
  takes over and the first one stops. Give parallel jobs their own names.
- **After a crash,** readings still in memory are lost. Do not send again what
  was already sent: it would be stored twice.

## Documentation

https://phloem.ai/docs/sdk/

## Support

Found a bug or need something? [Open an issue](https://github.com/phloem-labs/phloem-go/issues).

## Licence

Apache License 2.0. See `LICENSE`.

## Platforms in this release

- `darwin_amd64`
- `darwin_arm64`
- `linux_amd64`
- `linux_arm64`
