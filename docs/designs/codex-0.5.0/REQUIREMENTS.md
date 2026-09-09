# Codex integration requirements

Owner-authorized work, 2026-09-09. This register carries forward the Claude lessons; it does not select Claude's topology for Codex. `ANALYSIS.md` records evidence and unresolved choices. Implementation must satisfy both modes together.

1. **UMKA IS DISPOSABLE AND DEDICATED TO THIS DEVELOPMENT. NO INSTALLED VERSION IS PROTECTED.** Build, install, reinstall, fix and test the actual permanent installation, real home, native configuration, login PATH and Sessionbus service. No alternate CODEX_HOME, private product configuration or test-only installation prefix. Evidence directories contain records, not a substitute product environment. Coordinate the host writer. Do not interfere with the owner's sessions or shared tmux server.
2. Go implementation and public Go SDK. Improve that SDK where necessary. Node is permissible only for a required capability that cannot reasonably be implemented without it, with that necessity established first. Preserve the existing Go/Node implementations, native facts, bug fixes and regressions as knowledge.
3. Start with v0.1.0; compare v0.2.0, v0.3.0, 0.4.0 and 0.5.0 release by release. A historical passing row is useful evidence, not proof that every part of its architecture was correct. A later failure is not universal native impossibility.
4. One public `codex-peer` launcher, token-selected daemon worker mode, and private internal entry points only where needed. No separate public lane executable or product-specific lane skill. Ordinary `codex`, including plain nested invocations, remains ordinary.
5. One generic Sessionbus skill/public tool teaches peer messaging and lane describe/spawn/start/status/wait/interrupt/close/forget. Spawn/describe selects the product; subsequent operations use the returned session/run identifiers. The installed Codex parent itself must successfully operate a Codex lane through this tool.
6. Product owns its native flags, ordering, configuration, picker, name/ID resume selection, fork, permissions, errors and edge handling. Pass native arguments through in order. Document any required activation prefix. Do not add selectors, match sessions by cwd/time/name, infer titles, create native IDs, repair native history, synthesize work or substitute approval defaults.

   Owner clarification after the installed Claude failure: `-g` is an alias of `--group`. Consume repeated group options wherever they occur before native `--`, accumulating groups using the bus-wide list grammar; mixed short/long forms have the same meaning. They are not restricted to one trailing pair. Translate supported integration aliases: `--yolo` to the native product option; `--resume` to the native resume syntax where different (Codex's resume subcommand), while Claude's native `--resume` stays unchanged. Preserve the order and bytes of all remaining native arguments. Native `--` ends integration-option parsing. Do not build a native flag/arity table to implement these integration-owned options. The prior suffix-only contract was an integration design/review error, not a native product limitation.

   The selected interactive App Server transport has explicit limits, recorded in
   `INTERACTIVE-SELECTION.md`: native config `-c`/`--config` occurrences are
   mirrored in order to the owned server through a bounded single-option parser;
   all other flags remain TUI arguments. Caller `--remote` conflicts with the
   owned transport and is rejected explicitly. Native profiles remain TUI-only:
   the native App Server command rejects `--profile`, so server-level profile
   equivalence is not promised. Do not describe this managed remote architecture
   as unrestricted standalone configuration transparency. Native selection,
   picker, resume and fork remain native; no selector replacement is permitted.
7. Identity is only the settled identity reported by the product. Name may be absent until the product reports one. Launch groups use the bus-wide grammar and must reach the exact launch; a daemon-global value is not a substitute. A provisional selection must not become an addressable peer.
8. Exactly one live integration owner/connection for a peer or worker. Native identity changes and native termination update/withdraw the correct row. Presence mirrors a live product session, not TUI focus: Codex may keep a previous thread loaded and consuming after `/clear`, so that live thread remains a peer. Do not invent an unattended state or kill it to imitate another product. Use the daemon's existing same-ID rule; no integration lock or native-writer exclusion. Native external writers remain a product fact.
9. Sessionbus daemon remains product-independent. Every additional process, endpoint, connection, state field, lock, queue, wait and cleanup action needs a concrete invariant and native/daemon owner. Prefer removing machinery. No adapter timer, retry, polling, replay queue, ancestry registry or fallback delivery route.
10. Lane open completes only after native open, settled ID and required tool availability; it performs no model work. Preserve caller-selected native configuration. Failed or cancelled open cleans up what it started. Collection cancellation preserves the retained result. Terminal correlation, interrupt, EOF and close use native facts, not guessed outcomes.
11. Receipt strength is exact: `written` means local carrier write only; `injected` requires native admission acknowledgment; `queued_for_next_turn` requires native staging at that boundary; `accepted` promises retention. Uncertain submission is not a rejection and never gets automatically resent. No completion/consumption claim from admission alone.
12. Cleanup acceptance covers our worker, the native product process we own, and our MCP. Arbitrary native Bash/tool descendants are outside the owner's requested containment scope. Record native limits without adding parent-PID polling, sweepers or a cgroup project.
13. Reuse sealed evidence. New native execution answers named missing behavior or validates the actual installed candidate; it is not another broad discovery campaign. Source/fixture checks do not count as installed product acceptance. Preserve failures and scope results accurately.
14. Install/removal and activation are product features: normal paths, ordinary invocation, permissions, skill discovery, exact installed binary/assets, and uninstall/reinstall must work. Keep the evolving candidate installed for manual testing. Both interactive and lane regressions run against that same changed installation.
15. **PERSISTENT LANES AND AUTO-ARCHIVAL ARE PART OF THE CROSS-PRODUCT LANE CONTRACT.** Recover their exact earlier requirements before declaring the lane design complete. Persistence is an explicit lifetime choice. Independently, message-wake mode allows an incoming message to wake an idle receiver and start work without a separate caller `turn.run`. Ordinary caller-run lanes retain the no-unrequested-work rule. Do not silently substitute caller-run-only lanes for message-woken correspondents.

    **Owner clarification: persistence and auto-archive are orthogonal features.** Persistence controls owner-exit cleanup; auto-archive independently controls post-terminal retirement. Thus persistent lanes may auto-archive, and parent-owned lanes may disable auto-archive. Idle-message wake is a separate work-admission choice, not an implicit synonym for persistence.

    The design must explicitly identify who owns a message-started run and its output, how it uses the daemon's run slot, what completion/collection does to lane lifetime, and the earlier automatic archival policy versus explicit close/forget. Native history archival, terminating a running process, disconnecting presence, and consuming a retained result are different operations; do not conflate them. The historical default was 60 seconds after the latest terminal, cancelled by new work; collection did not trigger archival. Reconcile that explicit lifecycle policy with the later prohibition on adapter timers before implementation. This requirement applies to all products and is not an additional Codex attachment probe.

## Required installed outcome

Shared lifecycle prerequisite completed on 2026-09-09: Sessionbus PR48 merged at
`d11fa7433694c077f57b1f0af05a48325b464516`; the Go Claude adaptation and installed
acceptance merged in peers PR10 (`f5dd343`). Reuse the public Worker/cursor/policy
contract and its reviewed tests. Codex must implement its native seeded Run and
receipt boundary; it must not reintroduce Caller-local result state or its own
persistence/archive scheduler. The retained native Codex facts and unresolved
interactive topology questions remain unchanged by that shared repair.

Subsequent Codex selection is now recorded in `LANE-SELECTION.md` and
`INTERACTIVE-SELECTION.md`: one common native plugin/Go artifact, lane Worker
ownership, interactive broker ownership of each loaded native thread, and
stateless native MCP forwarders in both modes. The dynamic-tool comparison is
closed because arbitrary existing resume/fork cannot add its tool definition.
Selection is not runtime acceptance; the actual common umka installation still
has to demonstrate the required Codex compositions.

Interactive: ordinary invocation; integrated activation/skill/public tool; launch groups; native fresh/name/resume/picker/fork as supported; rename/clear/quit behavior according to native session lifetime (including older still-live threads); idle/active messages; native denial; own MCP loss/exit.

Lane: no-input open; native name/ID resume; caller configuration; native required tools; idle staging/active admission; correlated success/failure/interrupt; cancellation and later collection; close/EOF/failed open; two-session isolation and daemon same-ID rule; worker/native/our-MCP cleanup; actual Codex-parent generic-tool lifecycle. Compare each against retained earlier results, including known failures.
