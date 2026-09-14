> Historical contract notice (2026-09-09): the owner superseded the suffix-only
> group grammar below. Current parsing consumes repeated `-g` / `--group` before
> native `--` and translates `--yolo`; see [R12/R13](REQUIREMENTS.md) and the
> [installed correction](launch-flags-fix.md). Original probe evidence and the
> Node reference remain unchanged and predate this correction.

# Interactive activation amendment — B04/B06 and requirements

2026-09-09. **Architect selection submitted for stamp. Activation code remains held until fable releases it.** This amends only activation and affected packaging/tests in the stamped inventory SHA256 `154a594b57bb38fe68dac410d56ea03faea337c8ca7f9b9ca64f9d0478eb0681`, which remains byte-for-byte preserved. Independent implementation continues under its existing release; no new native run or installation is authorized here.

## Decision

`claude-peer` loads the complete installed package for that invocation using native `--plugin-dir`: skill, MCP and the three native hooks together. It supplies one exact origin-qualified native public-tool allow rule. It then execs Claude. Claude keeps the MCP owner; no dormant global server, activation marker, extra process, endpoint or settings merger is added. The daemon stays generic.

Ordinary `claude` receives no injected flag and is not activated by installing this package. Explicit user configuration can still load plugins; Sessionbus must not undo that configuration. In particular, preexisting globally enabled older integrations need explicit migration accounting, not silent disablement or a claim that our new package removed them.

AP01 at `293ebf3a31bf5f26d890d99281a6f986cd0ce3e1` proves this native route on 2.1.266: per-launch skill expansion, hidden reports, named/grouped hello and one public list call; exact native disallow omits the public tool and prevents handler execution. Ordinary launch has no fixture MCP/report/row, with its skill inventory limit retained. [Independent review](../../evidence/ap01r2-runtime-review/REVIEW.md) and [source/history](../../sources/activation-review-266/REVIEW.md) qualify every claim. Links resolve from the packet root via the references below if rendered outside this directory.

## Exact argv contract — explicit R12 amendment

After removing only the existing final `-g VALUE` suffix and exporting this launch's group JSON, exec the native executable with this array:

```js
[
  nativeExecutable,
  '--allowedTools', 'mcp__plugin_sessionbus_sessionbus__sessionbus',
  '--plugin-dir', absoluteInstalledClaudePackageRoot,
  ...nativeArgv
]
```

Resolve the package root from the installed main module, not cwd, a registry lookup or a native cache search. Every remaining caller argument is retained byte-for-byte and in its original order, including native `-n`, repeats, invalid options, empty strings, prompts and `--`. Native parsing and repeated-option precedence remain Claude's responsibility.

**Why prefix rather than the requested append:** a caller's native `--` makes appended flags positional prompt text; trailing variadic options can also change what appended text means. A prefix keeps activation outside the caller's end-of-options region without parsing or rewriting that region. Within the prefix, fixed-arity `--plugin-dir <path>` terminates the preceding variadic `--allowedTools <tools...>` so a bare caller prompt is not swallowed by our allow flag. Native 2.1.266 declarations are at byte192123708 and byte192132820 in the hash-bound binary. This exact production ordering is an implementation/test choice, not an argv ordering already run in AP01 (which put plugin-dir first and had native -n after the allow value). It must be included in final installed acceptance. Do not silently append, insert around `--`, maintain a native option table or normalize caller args.

The allow rule names exactly one plugin-origin public tool; no wildcard, hidden-handler grant, bypass flag, permission-mode override or global permission edit. An explicit caller disallow and managed/native policy remain authoritative. AP01 demonstrates successful callability under its exact read-only diagnostic configuration, not the sole granting rule or approval of every production action. Production `sessionbus` includes mutations and must not advertise readOnlyHint:true for the whole tool. Its native prompt/denial behavior remains native.

## Exact inventory delta

| File / element | Required change and invariant | Measured owner / earlier release comparison |
|---|---|---|
| `claude/main.mjs` | Add the fixed native prefix above to existing exec/group handling. Keep token-mode unavailable until lane implementation is selected. | Native loads the per-launch plugin in AP01; R4 used exec plus a global plugin and origin wildcard. No retained launcher is restored. |
| `claude/.mcp.json` | Server key `sessionbus`; command `node`; args `["${CLAUDE_PLUGIN_ROOT}/mcp.mjs"]`. The native plugin-root expansion binds installed payload. | AP01 used this expansion to entry.mjs. R2–R4 used a global runtime command. Production entrypoint is validated at step7. |
| `claude/package.json`, lock | Only public bin `claude-peer` points to main.mjs. Remove proposed second global MCP bin. Include runtime, dot-manifests, hooks and skill in npm pack; retain pinned kit and Node prerequisite. | MCP is native-spawned from the package root, so a second public command has no invariant to serve. |
| `claude/.claude-plugin/plugin.json`, `claude/hooks/hooks.json` | Keep plugin name `sessionbus`, common hidden mcp_tool UserPromptSubmit/Stop/SessionEnd hooks only. No SessionStart and no command hook for interactive. | AP01 loads the hook set per launch. R2–R4 had no Claude hooks; global/inert MCP alone did not need to solve pre-call hook errors. |
| `claude/mcp.mjs`, `owner.mjs`, `delivery.mjs`, `tools.mjs` | Retain selected ownership, hidden handler, in-process tools, Connection/Caller and written/uncertainty semantics. No activation dummy or global-mode detector. Correct public annotations. | AP01 uses intended Owner/delivery bytes; full transport/public package still needs candidate acceptance. |
| `claude/skills/sessionbus/SKILL.md`, existing doctor guidance, Claude/root README | Describe explicit integrated launch, actual qualified tool, native permission/denial, ordinary launch and partial lane status. Skill is bundled for per-launch discovery; explicit slash invocation is proven. | R1 supplied global skills without MCP; R2–R4 intentionally used global/inert MCP. No marketplace is required for AP01 skill loading. |
| Existing six tests, architecture test, `INTERACTIVE-SIZE.json` | Add exact prefix and package-root/bin/manifest assertions; retain all existing functional tests and whole-package accounting. No new runtime module or test-helper service. | Five runtime modules remain; neither moving the MCP entry into the package nor changing installation makes dependencies disappear. |

No additional runtime file is selected. Existing Go interactive deletions, held lane source, CI and independent daemon requirements remain as stamped. Root marketplace metadata may remain repository metadata; the new README does not install it for Claude activation.

## Install, use, remove

The candidate recipe becomes `npm ci --prefix claude`, then `npm install --global ./claude` from the recorded checkout, with compatible separately installed Sessionbus and documented Node/npm/Claude prerequisites. **No native marketplace add or native plugin install is part of this recipe.** The launcher points native plugin-dir at its installed npm package root; native discovers its skill/hooks/MCP there. Bind npm pack contents, executable resolution, installed module path and native-loaded payload at step7.

Use `claude-peer -n NAME -g GROUP`; resume with native `claude-peer --resume NAME -g GROUP`. Existing first-report/no-deadline, zero-turn history and written-only cost sentences remain. Add these sentences:

> Installing this package does not enable it globally in Claude. Run claude-peer to load its skill, hooks and communication tools for that session. Ordinary claude remains ordinary unless you explicitly configure a plugin yourself. Claude's own policy can deny a Sessionbus tool; the integration does not bypass it.
>
> A plain nested claude process does not receive the launcher's plugin flag and is not automatically a peer. Use claude-peer explicitly for a child that should participate, with that launch's groups. Sessionbus does not intercept or rewrite Bash commands.

Removal for this recipe is `npm uninstall --global @sessionbus/claude`; no native uninstall is applicable to its unregistered per-launch plugin directory. Preserve native history/config, other plugins and the daemon. Record any preexisting global Sessionbus/legacy plugin separately and use native migration/removal only with its exact ownership established. No general cache/config-restoration claim.

FP05's earlier nested-peer pass used a globally installed fixture. It remains valid for that topology, not proof that flags are inherited. Under R06, plain unconfigured nested Claude is intentionally ordinary. Explicit child integrated launches retain per-launch group rules (no inherited default when -g is absent). User-configured globally loaded plugins still behave as Claude dictates. Lane whole-plugin loading/forwarding and native child configuration remain OB03 obligations; this amendment accepts neither lane behavior nor a mixed old-Go bridge.

## Implementation and final acceptance

After stamp, dev1 finishes existing Phase C steps1–6 with this delta and updated REQUIREMENTS. Deterministic production tests cover prefix before literal --, bare prompt, repeats, empty/invalid values, exact final group suffix, caller allow/disallow preserved, package root containing spaces, direct MCP payload resolution, pack contents and no marketplace-install requirement. No test reimplements Claude's native parser or claims it proves native permissions.

The separately released step7 uses the actual installed launcher/prefix and production MCP/skill/public tool, includes plain native versus integrated activation and caller denial, and performs the full named/resume/communication/removal path. Record native outcomes under real policy; don't apply the diagnostic's read-only permission claim to the production mutating tool. No additional architectural probe is requested by this amendment.

## Historical comparison and evidence references

| Release / exact commit | Prior activation and reason to keep/remove it |
|---|---|
| R1 `83e8ff3fe69f3bb46f1c6e7c8a7f3bf7efceb04e` | Global skill plugin, no MCP/hooks/grant. No evidence that R1 needed a dummy MCP. |
| R2 `cafce25ec0239a8c743ca90c6bb8bf9c5b8e044a` | Global MCP, inactive when launch attestation fails, no Claude hooks. Deliberate separation of installed capability and participation. |
| R3 `679fe9d3068b6362df867f8d78ce6708c4ce1342` | Global/inert MCP, no hooks; per-launch profile and four exact origin-qualified grants. Preserves native permission ownership but adds profile machinery absent from this selection. |
| R4 `ff81565a252151575409efad2fccfd6d5e544383` | Global/inert MCP, no hooks; exec launcher injects origin wildcard. Source differs from retained README's four-tool overlay wording. Current one-tool exact allow is narrower. |
| Original R5 peers `21dea989ac700ec5ba072983a155f17706c7f381` | Suspect Go interactive path is being removed as inventoried; it is not the activation authority. |

The earlier global/inert design was purposeful, not shown impossible. It does not by itself make new global report hooks harmless: FP01 proved hooks can fail before MCP callability, before any dummy handler could act. AP01 demonstrates the simpler whole-plugin per-launch route, including the required skill. This is why it is selected.

Packet references: `sources/activation-review-266/REVIEW.md`, `evidence/ap01-source-review/REVIEW.md`, `evidence/ap01r2-source-review/REVIEW.md`, `evidence/ap01r2-runtime-review/REVIEW.md`, and the sealed AP01 ledger at commit293ebf3. All original stamped files and failed probes remain preserved.
