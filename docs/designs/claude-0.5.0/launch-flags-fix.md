# Claude launch flags — owner correction, 2026-09-09

The earlier suffix-only grammar was too narrow. `claude-peer --resume test1
-g test,test2 --yolo` forwarded `-g` to Claude because it was not the last pair.
The owner clarified that wrapper options may occur anywhere before native `--`.

Source `569ce9f8a8cff524d452b415f5a4ff0b80f93951` consumes repeated `-g VALUE`,
`--group VALUE` and `--group=VALUE`, accumulating comma lists in order under the
existing bus grammar. It translates `--yolo` to Claude's native
`--dangerously-skip-permissions`. Native `--resume` needs no translation.
All unused arguments retain their order/bytes. Native `--` terminates wrapper
parsing. There is no native option arity table, selector or CLI dependency.

The permanent umka package was replaced through the archive README recipe.
Installed binary SHA256:
`bf7bdebca086726b9982cf0c315dd15c4230eed01be4c28710dea0a1c7768faf`.
Native Claude was 2.1.266, actual executable SHA256
`19842705e989393fce936804df6d2ab034860e24b8f8880357981d87ffd83fac`.
Before/after unit, daemon and configuration hashes are recorded in the install
seal; installation did not alter those files.

Installed results:

- Both exact reported command forms, and the mixed repeated-group form, with
  `--version` appended returned native 2.1.266, exit 0 and empty stderr.
  This deliberately avoided selecting the owner's `test1` session; it does not
  claim a new successful resume observation.
- A disposable fresh launch used `--group=flags-extra`, `-g test`,
  `--group test2,test3` and `--yolo`. Actual native argv contained the fixed
  activation prefix, native name and `--dangerously-skip-permissions`, with no
  wrapper group arguments. Its environment held the four groups in that order.
- One native public Sessionbus `list` call returned its own connected row,
  `74744734-53a7-4036-8db8-f01a428d0c7a@local`, with those same four explicit
  groups plus its daemon private group. An independent control peer observed
  the same row. The native assistant replied exactly `CLAUDE_FLAGS_OK_20260909`.
  A native ToolSearch also occurred; no exclusive-tool claim is made.
- `/quit` exited 0 and was reaped. Native PID 1747788, observer 1747785 and
  observed MCP 1747834 were absent at the final checkpoint. The later roster
  contained no test Claude row. This is not a SessionEnd/EOF causal claim.

Evidence on umka and its pdev copy:
`/home/antst/sessionbus-evidence/claude-launch-flags-r2-20260909/raw/fresh/`
(`FINAL-SHA256SUMS`, `native-argv-groups.json`, `versions.json`,
`transcript-final.json`, `roster-0.json`, `roster-1.json`, `final-pids.json`).
Install seal: umka
`/home/antst/sessionbus-evidence/claude-launch-flags-install-569ce9f/`, copied to
the r2 evidence root's `install/` on pdev. Captures project explicit fields only.
The reused projector has a duplicated marker in its exact-marker list;
the independent count and text hash describe one reply, not two.

The earlier 7fede10 install/zero-input launch was superseded when the owner
clarified the long alias. It received no prompt and quit 0/reaped; a subsequent
pane-removal attempt returned 1 after the pane had ended. Those records remain
under `claude-launch-flags-20260909`; they are not the mixed-alias acceptance.

Offline tests cover the reported argv, mixed/repeated positions, unknown native
flags and values, unchanged native `--` operands, missing group values, and
compiled installed-launcher PID/argv/environment behavior. Existing lane and
interactive race regressions remain applicable; no runtime lane logic changed.
