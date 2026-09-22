# Stable functionality checklist — Codex separation

Baseline: original peers main `710e5d33369cba4fb9468cd24fea0fe844a0219d`.
The checklist IDs below are stable. A requirement is not removed or reworded to
make a failed check pass. Corrections must state the original claim, source
reason, behavior impact and remaining gap. Native unsupported behavior stays
explicit, not silently implemented or counted as a pass.

Status for this extraction: **local verification in progress; installed checks
pending**. Historical Codex wake acceptance is 4/4, separately recorded below.
Passing old evidence or unit tests alone does not complete this migration.

Each result must identify source revision, artifact hash, native version,
permanent installation and whether it is controlled-test, historical or fresh
installed evidence. Fresh tests use one send with no automatic replay and no
post-inbound model prompt. Tests stop and preserve their first failure.

| ID | Preserved functionality | Existing regression coverage | Installed check / evidence limit |
|---|---|---|---|
| F01 | Complete install/update/uninstall, native plugin/skill/private aliases, checksum and archive safety | wrappers/codex/package*_test.go; scripts/release/*_test.go | Fresh archive install + reinstall in real HOME; confirm only owned removal |
| F02 | One public command, version/native-version and token-selected lane mode | cmd/codex-peer/main_test.go; version_test.go; host/launch_test.go | Permanent command and private aliases report exact revision |
| F03 | Native argv/order/--, repeated groups/name, resume and yolo aliases | arguments_test.go; alias_values_test.go; interactive_options_test.go | Same native settings/identity and retained CLI behavior |
| F04 | Native default approvals/sandbox; scoped Sessionbus grant; reject defeating overrides | arguments_test.go; package_test.go; codex_test.go | Normal-policy Sessionbus call; native unrelated policies remain |
| F05 | Ordinary native launch stays ordinary | package_test.go; interactive_launch_test.go | Zero-input plain Codex has no managed peer/Sessionbus activation |
| F06 | Interactive identity/title/groups, rename/fork/clear, native resume | peer_test.go; broker_owners_test.go | Exact native identity/public row join; resumed history preserved |
| F07 | Discovery/self_info, single/multiple/group messaging and error semantics | wrappers/mcp/*_test.go; public SDK fixtures | Actual native list/send with authenticated receiver correlation |
| F08 | No-input lane open, native/tool readiness, spawn/resume | codex_test.go; lane_lifetime_test.go | No model input until Run; exact native lane identity |
| F09 | Run/start/status/wait/ack, result cursor, exact outcome, interrupt | codex_test.go; wake_worker_test.go; wrappers/mcp/lane_test.go | Collect native results before ack; distinct managed runs; resume retains history |
| F10 | Parent lifecycle operations, completion notification, direct-child trace | wrappers/mcp/trace_policy_test.go; SDK behavior; prior parent acceptance | Native parent operations retained; tracing authority remains daemon-owned |
| F11 | Interactive idle inbound autonomously wakes | peer_test.go; broker_owners_test.go | One inbound -> exact native reply + final, no post-send model input |
| F12 | Interactive active inbound admission and later processing | peer_test.go; broker_owners_test.go | Original active identity/process witness -> receipt -> exact reply/final |
| F13 | Managed idle inbound autonomously wakes | wake_worker_test.go; staging_test.go | One inbound -> automatic managed turn/native reply/final |
| F14 | Managed active admission/queue, exact current-turn guard | codex_test.go; staging_test.go; wake_worker_test.go | Current turn/pointer unchanged; valid steer or definite not-submitted queue; no replay |
| F15 | Daemon reconnect while interactive owner lives; no worker resurrection/replay | peer_test.go; broker_owners_test.go; SDK fixtures | Retain native owner and latest identity; outages are not queued replay |
| F16 | Cancellation/backpressure/protocol bounds/error fidelity | app*_test.go; broker_mux_test.go; broker_transport_test.go; wrappers/mcp/*bounds*_test.go | Controlled race tests are evidence, not invented native outcomes |
| F17 | Normal exit/failed Open/forced native death and owned-process cleanup | lane_lifetime_test.go; broker_watch_test.go; interactive_launch_test.go | Owned rows/processes/endpoints absent; unrelated processes preserved |
| F18 | Native history retained; persistence/auto-close independent and daemon-owned | codex_test.go; public SDK fixtures; prior lifecycle acceptance | Close does not delete native history; no local scheduler added |
| F19 | Independent package/module/CI/release and four-platform build | architecture_test.go; package_archive_test.go; release tests; CI | Actual extracted artifact verified; GitHub App/publisher bindings tracked separately |
| F20 | All original Codex/common runtime, fixtures and regression tests preserved | PRESERVED-FILES.json + original commit; test inventory | Only mechanical imports and product-specific build metadata change |

Coverage paths without a prefix are under `wrappers/codex/`. Existing tests are
retained in full; this table groups them instead of substituting new shallow tests.
The full frozen file/test inventory is in [PRESERVED-FILES.json](PRESERVED-FILES.json).

## Fixed limitations

- Native profiles are TUI-only where the remote protocol omits them.
- Receipts prove only their documented admission strength, not durable storage
  or model consumption. `no_receipt` is uncertain and must not trigger replay.
- Interactive `running:false` is not an idle-model witness.
- Cleanup covers owned processes; arbitrary tool-descendant containment is not
  claimed. Prior broker-death stale-socket behavior remains explicitly documented.
- Darwin build/tests are not a claim of installed Linux observations on Darwin.
- The plugin's native `codex@sessionbus-peers` identity and private aliases stay
  unchanged. The released Bus SDK module identity stays unchanged.

## Historical installed wake baseline

Source `1989c52d9cb5e74b40985ceba67841700a5b05d4` is included in baseline main.
Native Codex 0.153.4; these results belong to their original binaries/daemon and
must not be relabelled as the extracted build:

| Surface | Accepted evidence manifest SHA256 |
|---|---|
| Lane active O | cfdbb713958655ad6b4cd760a0a75e75fc2a6f906f1e0a03d63acab5cb61fefd |
| Lane idle P | b1ad66af6d752f218a9aa5ec0809077af191a30ee5f65b7b91bbe2c1ee32834e |
| Peer idle R | 8920967f850407d12a48e2cbb4f4700ccef9c0dfc6b466271fc58095ac68920d |
| Peer active S | 29ae397c99460df57795989f9a125804eac96ecfd2dbacffb16df94fd860682d |

Packets: `/home/antst/sessionbus-evidence/codex-message-wake-installed-dev1-20260921`.
Broader historical lifecycle/installer/parent/native-death observations and
qualifications remain in `docs/designs/codex-0.5.0/ACCEPTANCE.md`.

## Completion rule

1. Account for every original product runtime/asset/test file and dependency.
2. Preserve runtime behavior; any discovered functional fix is a separate change.
3. Run retained tests, race checks, vet/lint, installer/archive and platform checks.
4. Install the exact artifact permanently on UMKA and verify both interactive
   and lane behavior with the established evidence collectors.
5. Record failures and limitations without weakening requirements, then review
   the concrete diff/results before publishing or advancing to the next product.
