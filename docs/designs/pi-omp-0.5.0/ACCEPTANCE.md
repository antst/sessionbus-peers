# Pi and OMP acceptance status

This record distinguishes installed native evidence from deterministic fixtures.
It accompanies [DESIGN.md](DESIGN.md); a successful replay of preserved native
records establishes the stated runtime behavior even when the original fixture
made an incorrect assertion. Such fixture failures remain failures and do not
require another model call solely to produce a clean fixture exit.

## Established installed behavior

| Product | Behavior | Evidence scope |
| --- | --- | --- |
| Pi | Worker Open/Close and persisted interactive resume/Ctrl-D | Current `19f1b1b` regression: exact identities, process joins, four byte-identical history copies |
| Pi | Normal Run, Sessionbus list/self identity, current final result | Installed `e946fb1` native/public correspondence |
| Pi | Idle staged delivery, explicit Run, Forget | Installed `399de2f`: staged envelope submitted once; bus row removed; native history retained |
| Pi | Interactive idle delivery and native session replacement | Installed `aa096e8`: two written receipts, distinct native identities in the same process, normal Ctrl-D |
| Pi | Busy interactive delivery and settled drain | Native/raw replay verified; original fixture failed its pre-Hello protocol assumption. Later idle-write step was not attempted; cleanup used TERM |
| OMP | Worker Open/Close and ordinary interactive startup/Ctrl-D | Installed `332471c`: identities, direct Bun ancestry and joined resources |
| OMP | Normal Run and Sessionbus list/self identity | Installed `332471c`: core native/public result verified through default xdev dispatch. Original fixture failed its top-level-tool assertion and had a secondary closed-stream capture error |

Root verification records are retained under
`/home/antst/pi-omp-architecture-20260911`: `review-pi-startup-ui-zero-installed-root`,
`review-pi-normal-run-installed-root`, `review-pi-delivery-forget-installed-root`,
`review-pi-interactive-installed-root`, `review-pi-interactive-busy-installed-root`,
and `review-omp-normal-run-first-outcome-root`. The exact OMP zero-input capture
is `installed-omp-zero-open-close-socket-332471c-dev1` in that directory.

## Deterministic coverage already available

The integrated tests cover native transport loss and cancellation, current-result
attribution, no replay after uncertain submission, and joined cleanup. Specific
regressions include Pi append cursor separate from selected history leaf,
cross-channel start/settled reconciliation, and UI writes releasing admission
locks before waiting. OMP tests cover no-agent results without borrowed output,
interrupt followed by healthy reuse, staged delivery interrupted before preflight,
UI cancel/write joins, replacement token rotation and child binding cleanup.
These are fixture authorities, not installed native acceptance claims.

Pi error/local-only and loss cases retain the deterministic scope described by
the original remaining-acceptance inventory. Additional native runs should add
an observable product boundary rather than repeat the same fixture schedule.

## Remaining release work

- Pi: installed interruption of an admitted Run followed by a healthy next Run.
- OMP: installed idle lane staging, one explicit Run and Forget with native history
  retention; then interactive delivery/context and replacement boundaries.
- OMP: assess the installed dialog/no-agent/child boundaries against the exact
  deterministic evidence before selecting further native scenarios. These remain
  qualified until the decision and any required evidence are recorded.
- Finish existing-product regression, final archive/install correspondence and
  CI on the integrated release source before merge. Linux native acceptance does
  not establish Darwin native behavior; report that platform separately.

## Native environment limits

Ambient extensions and native configuration remain enabled. An extension that
writes unrelated bytes to OMP RPC stdout can retire the lane as invalid framing;
the wrapper does not silently suppress it. Native startup can migrate legacy
settings or keybindings and update native bookkeeping. Preserve the observed
configuration deltas rather than claim blanket content equality. Native titles,
model selection and permissions remain subject to the source-bound design.

Historical product fact sheets retain their original citations. Their old
preimplementation questions are superseded by this record and the current
design; they are not additional requests to repeat completed acceptance.
