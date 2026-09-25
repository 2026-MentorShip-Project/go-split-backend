# Goal

Build a minimal continuous CPU profiler for the Go service running on Cloud Run, specifically to generate representative production profiles for Go PGO (Profile-Guided Optimization).

The goal is not general observability or debugging. We want to answer:

    “Which Go code paths are actually CPU-hot under our real production workload, and can we feed that profile into go build -pgo?”

# Key design

```
Cloud Run instances
       │
       │ periodically collect CPU profiles
       ▼
   pprof CPU profile
       │
       ▼
   profile storage
       │
       │ merge representative profiles
       ▼
   default.pgo
       │
       ▼
go build -pgo=default.pgo
       │
       ▼
 optimized binary
```

Pyroscope is optional. The experiment can be implemented with Go's native runtime/pprof and a small uploader. The Pyroscope SDK is essentially providing the collection/upload infrastructure for this.

# TODOs
1. Build a minimal CPU profiler

    Add runtime/pprof

    Start CPU profiling periodically

    Capture e.g. 60-second samples

    Stop profiling

    Produce the standard pprof profile

    Run this in a background goroutine

Initial cadence could be:

60s profile
every 5–15 minutes

Tune later based on overhead and profile quality.
2. Push/store profiles

Initially keep this extremely simple.

    Decide where profiles go

    Upload the .pb.gz profile

    Include metadata:

        service

        environment

        Cloud Run revision

        region

        application version

No need for heap/goroutine/mutex/block profiles for the PGO use case.
3. Make the profile representative

This is probably the most important TODO.

    Ensure profiles capture normal production traffic

    Avoid collecting during unusual workloads/deployments

    Capture across multiple Cloud Run instances

    Collect enough samples to represent the normal request mix

    Eventually merge profiles from multiple instances/time periods

instance A ─┐
instance B ─┼─> representative merged CPU profile
instance C ─┘
                       │
                       ▼
                  default.pgo

4. Generate PGO profile

    Merge representative CPU profiles

    Produce default.pgo

    Build with:

go build -pgo=default.pgo

    Deploy the PGO build to Cloud Run

5. Benchmark the result

Compare before vs after PGO using the same workload.

Track:

p50 latency
p95 latency
p99 latency
CPU utilization
requests/sec

And ideally a controlled load test rather than relying solely on production traffic.
6. Validate that PGO is actually helping

The final loop is:

production
    ↓
CPU profile
    ↓
PGO
    ↓
new binary
    ↓
benchmark
    ↓
production
    ↓
new profile
    ↓
repeat

Explicitly out of scope for the first version

Don't build these yet:

    ❌ Heap profiling

    ❌ Allocation profiling

    ❌ Goroutine profiling

    ❌ Mutex profiling

    ❌ Block profiling

    ❌ Full Pyroscope deployment

    ❌ General-purpose profiling dashboard

Those are useful for performance debugging, but they're not necessary for your immediate PGO pipeline.

MVP = runtime/pprof CPU → store → representative merge → default.pgo → benchmark.