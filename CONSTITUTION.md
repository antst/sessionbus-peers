# Constitution of sessionbus-peers

These rules bind every design, review, stamp, and merge in this repository.
Each one exists because a release shipped without it and broke on first use.
A pull request that violates a rule is rejected on sight, whatever else it
does well. Changing this document requires the owner's signature.

## 1. The user's command is the contract

The product is what a user types: `<product>-peer -n NAME -g GROUP`, the
product's own resume, the README install line. If those do not work, nothing
else counts.

- Every wrapper has a **first-contact matrix**, run before any stamp and
  before any release: install by the README on the real host → launch with a
  name and a group → the roster shows that name and those groups → quit →
  resume by that name through the product's own mechanism → receive a message
  from another peer → remove by the product's own mechanism.
- The matrix is run by someone who did not write the wrapper, through the
  user's own commands, in the user's own shell. The evidence is the terminal
  transcript and the roster rows. Nothing else is evidence of readiness.
- A harness that starts the product itself with a prepared configuration and
  drives the helper or the API tests the engine. It never tests the front
  door, and its passes are never reported as product readiness.

## 2. The real environment is the only environment

Product tests run on the dedicated test host in the real home, the real
product configuration directories, the real login shell and `PATH`, against
the daemon installed as the real user service, with products installed the
way a user installs them.

- No isolated `*_HOME`, no evidence-owned prefixes, no synthetic environment,
  no private terminal multiplexer standing in for the user's terminal.
- Cleanup is by the product's own uninstall or by the edit a user would make.
  Never by deleting a throwaway home.
- The daemon and the products are decoupled on purpose: the daemon is
  installed once and stays; every product is installed, reinstalled, and
  removed independently against it. That decoupling is what makes sandboxes
  unnecessary.

## 3. Product facts before design, product behaviour over convenience

A wrapper is designed from `docs/products/<product>.md`, and that file is
written by a non-author from runtime probes on the real host.

- Every surface the wrapper will touch is probed in both modes, interactive
  and headless, with a seal per surface. A design citing one surface is not
  stamp-ready.
- A route or event described by the product's documentation or OpenAPI is not
  a fact until a runtime probe seals it.
- Never begin a fix by assuming the product behaves as it would suit us. Probe
  first, in the real environment, with zero model turns where possible. If the
  product exposes no relation the design needs, that is recorded as the
  blocker; it is never disguised as timing, ordering, or newest-wins.
- Facts rows are cited: product version, host evidence path, and legacy code
  by full commit on the Forgejo `legacy-*` branches.

## 4. Never reimplement what the product already does

- No sessionbus-side selector of any kind, for any product: no resume picker,
  no session list, no matcher, no fuzzy search, no "find by our name".
- Native flags pass through verbatim. A wrapper never intercepts, narrows, or
  refuses a native flag the product accepts. `--resume` is the product's.
- `-n NAME` becomes the product's **native** session name wherever the product
  has one, so the product's own resume finds it. Where the product has no
  native name, the README says how that product resumes and the wrapper adds
  nothing.
- A test that asserts a refusal contradicting the signed design is a defect
  with a test, not a tested behaviour.

## 5. The launcher owns identity

- Name and groups come from the launch command, for every product, every
  launch. No identity is configured at install time, and changing a group
  never requires reinstalling or restarting anything.
- Identity is published from the process that can see the launcher's
  environment. A helper the product spawns from installed configuration is a
  tool hop; it does not own the hello.
- An explicit request that cannot be honoured fails visibly. A wrapper never
  silently substitutes defaults for what the user asked.

## 6. Use what five releases already learned

- Previous releases are evidence, never a base to copy. Each of them carried
  its own breakage, which is why the rewrite was a clean cut. Before changing
  a path, find the last release in which that behaviour demonstrably worked
  (Forgejo `legacy-*` branches, cells, facts), state what it did, state what
  was wrong with it, and carry over the proven shape and the product facts,
  not the code.
- A fix changes the one thing that is wrong and touches nothing else. Working
  paths are not rewritten to make a fix convenient. "Break it all, fix half,
  break the other half" is the failure mode this rule exists to end.
- Findings are closed by removal or reshaping, never by adding checks, retries,
  timers, or fallbacks. There are no timers beyond named constants.
- If an edge is found, step back and redesign without the edge.

## 7. Authorship, review, stamp

- Every unit: one author, one non-author reviewer, one stamp. Nobody reviews
  or stamps their own work.
- A stamp requires: the first-contact matrix green in the real environment
  (Rule 1), the size ledger within the product's cap with net-negative diffs
  expected, no open finding, and every claim in the change traceable to a
  facts row or a file in the tree.
- Errors are truthful and name the cause. A message that blames a policy the
  design never had is a defect.

## 8. What this repository is

Wrappers, peer commands, plugins, installers, product facts, and skills for the
supported products, built only on the public sessionbus SDK. Each product is
supported only with full parity in both modes, peer and lane. Facts-only
products are not supported products. Installers own their removal.
