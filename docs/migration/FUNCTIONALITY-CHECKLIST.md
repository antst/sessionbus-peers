# Stable functionality checklist — Codex separation

Baseline: original peers main `710e5d33369cba4fb9468cd24fea0fe844a0219d`.
The checklist IDs below are stable. A requirement is not removed or reworded to
make a failed check pass. Corrections must state the original claim, source
reason, behavior impact and remaining gap. Native unsupported behavior stays
explicit, not silently implemented or counted as a pass.

Status for this extraction: **merged after source preservation, controlled checks,
permanent installation and four fresh installed wake checks passed**. Historical
results remain separate below; the new evidence does not relabel them.

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
| F07 | Discovery/self_info, single/multiple/group messaging and error semantics | pinned peer-common/mcp/*_test.go; public SDK fixtures | Actual native list/send with authenticated receiver correlation |
| F08 | No-input lane open, native/tool readiness, spawn/resume | codex_test.go; lane_lifetime_test.go | No model input until Run; exact native lane identity |
| F09 | Run/start/status/wait/ack, result cursor, exact outcome, interrupt | codex_test.go; wake_worker_test.go; pinned peer-common/mcp/lane_test.go | Collect native results before ack; distinct managed runs; resume retains history |
| F10 | Parent lifecycle operations, completion notification, direct-child trace | pinned peer-common/mcp/trace_policy_test.go; SDK behavior; prior parent acceptance | Native parent operations retained; tracing authority remains daemon-owned |
| F11 | Interactive idle inbound autonomously wakes | peer_test.go; broker_owners_test.go | One inbound -> exact native reply + final, no post-send model input |
| F12 | Interactive active inbound admission and later processing | peer_test.go; broker_owners_test.go | Original active identity/process witness -> receipt -> exact reply/final |
| F13 | Managed idle inbound autonomously wakes | wake_worker_test.go; staging_test.go | One inbound -> automatic managed turn/native reply/final |
| F14 | Managed active admission/queue, exact current-turn guard | codex_test.go; staging_test.go; wake_worker_test.go | Current turn/pointer unchanged; valid steer or definite not-submitted queue; no replay |
| F15 | Daemon reconnect while interactive owner lives; no worker resurrection/replay | peer_test.go; broker_owners_test.go; SDK fixtures | Retain native owner and latest identity; outages are not queued replay |
| F16 | Cancellation/backpressure/protocol bounds/error fidelity | app*_test.go; broker_mux_test.go; broker_transport_test.go; pinned peer-common/mcp/*bounds*_test.go | Controlled race tests are evidence, not invented native outcomes |
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

Shared source/test paths are resolved by `current_location` in
`PRESERVED-FILES.json`, against the exact common commit and module checksum.
The 69 common tests run in that repository; all Codex-specific tests remain here.

## Fresh extracted-build acceptance

Tested source: `a9dda5d7290fc1c22366c0821ae2d403d30fd651`; extraction merged as
`a9977bde52ae018a051c7dbb7d78f93362ba7941` with the identical tree. Shared module:
`github.com/sessionbus/peer-common v0.0.0-20260922143100-eb655f686e44`.
Linux-amd64 archive SHA-256:
`20942e4d29972ed1295b6874452a631bc88e9d154fa1464999737b53d1206751`;
installed wrapper SHA-256:
`ec7e9803abe67ed4dd824feccf20b83cc64db6e26e379b39a755fd6ffdbc3e60`.

Permanent install and idempotent reinstall evidence seal:
`cf4496b2c8aedbe074d07dc2e2dd4a5c0262df68afb4fbef45369cfbc620e980`.
Native Codex subsequently updated from 0.153.4 to 0.155.1; wrapper, common pin,
config and plugin remained unchanged. The refreshed test adapter is sealed as
`abf020e0985e0ae87ce13db005bfa53b1814e1c1ef314976494326f5e337dc71`.
Exact native versions/hashes identify what was tested, not a runtime version
allowlist. Users are expected to update native clients routinely.

| Requirement | Fresh cell on native 0.155.1 | Evidence manifest SHA-256 |
|---|---|---|
| F13 managed idle | CSW922B | 08f08835769ff92d6efb601ef2fb85051d5c6c31c34f66de09c944ebf0096d9c |
| F14 managed active | CSW922C | b7853e2218b80795af71385cc2e4325b4d8ca0558af7863552913109335ff6e6 |
| F11 interactive idle | CSW922D | 4544c8f42cd1607dd92f3e03d35aadb2e74075506d4871f56221835bc247cda3 |
| F12 interactive active | CSW922E | 563e0eabb34c057516c6324a42fb56ab6f8924a24c709f150bbaa7082ece29d1 |

Dev2 independently accepted all four raw packets without findings. Each binds
one inbound, the exact authenticated native envelope, one correlated Sessionbus
reply, exact final and completed terminal, no post-inbound harness model input
or replay, and owned cleanup. Idle cells start an automatic turn; active cells
inject into the original turn under the existing Codex contract. The initial
CSW922A usage-limit failure remains a separate zero-inbound diagnostic, not a
wake result. Packets live under
`/home/antst/sessionbus-evidence/codex-split-wake-installed-dev1-20260922`.

These fresh tests directly cover F11–F14 and their identity, communication,
terminal and cleanup joins. Broader F01–F20 coverage retains the controlled and
historical qualifications in the table; no claim is made that every feature was
retested with a live model. Source preservation, retained tests/race/vet/lint,
hosted Linux/macOS checks and four-platform archive checks passed. Release
publication remains held. See [PR61](https://github.com/sessionbus/codex-peer/pull/61)
for the source reviews and installed acceptance record.
