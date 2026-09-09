// SPDX-License-Identifier: MIT

package codex

import (
	"fmt"
	"strings"
)

type interactiveOptions struct {
	native, config, groups []string
	name                   string
}

// parseInteractiveOptions knows only wrapper flags and the explicitly selected
// -c value exception. Native owns every other option's interpretation.
func parseInteractiveOptions(args []string) (interactiveOptions, error) {
	out := interactiveOptions{groups: []string{}}
	addGroups := func(value string) {
		if value != "" {
			out.groups = append(out.groups, strings.Split(value, ",")...)
		}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			out.native = append(out.native, args[i:]...)
			return out, nil
		case arg == "-c" || arg == "--config":
			out.native = append(out.native, arg)
			out.config = append(out.config, arg)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				out.native = append(out.native, args[i])
				out.config = append(out.config, args[i])
			}
		case strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-c") && len(arg) > 2:
			out.native = append(out.native, arg)
			out.config = append(out.config, arg)
		case arg == "--remote" || arg == "--remote-auth-token-env" || strings.HasPrefix(arg, "--remote=") || strings.HasPrefix(arg, "--remote-auth-token-env="):
			return out, fmt.Errorf("%s conflicts with codex-peer's per-launch App Server transport; use native codex for a caller-owned remote", arg)
		case arg == "-g" || arg == "--group":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return out, fmt.Errorf("%s requires a group list", arg)
			}
			i++
			addGroups(args[i])
		case strings.HasPrefix(arg, "--group="):
			addGroups(strings.TrimPrefix(arg, "--group="))
		case strings.HasPrefix(arg, "-g") && len(arg) > 2:
			addGroups(strings.TrimPrefix(strings.TrimPrefix(arg, "-g"), "="))
		case arg == "-n" || arg == "--name":
			if i+1 == len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				return out, fmt.Errorf("%s requires a nonempty name", arg)
			}
			i++
			out.name = args[i]
		case strings.HasPrefix(arg, "--name="):
			out.name = strings.TrimPrefix(arg, "--name=")
			if out.name == "" {
				return out, fmt.Errorf("--name requires a nonempty name")
			}
		case strings.HasPrefix(arg, "-n") && len(arg) > 2:
			out.name = strings.TrimPrefix(strings.TrimPrefix(arg, "-n"), "=")
			if out.name == "" {
				return out, fmt.Errorf("-n requires a nonempty name")
			}
		case arg == "--resume":
			out.native = append(out.native, "resume")
		case strings.HasPrefix(arg, "--resume="):
			out.native = append(out.native, "resume", strings.TrimPrefix(arg, "--resume="))
		case arg == "--yolo":
			out.native = append(out.native, "--dangerously-bypass-approvals-and-sandbox")
		default:
			out.native = append(out.native, arg)
		}
	}
	return out, nil
}
