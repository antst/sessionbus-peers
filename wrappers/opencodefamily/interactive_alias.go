// SPDX-License-Identifier: MIT

package opencodefamily

import (
	"errors"
	"slices"
	"strings"
)

// OpenCodeResumeAlias preserves the reviewed OpenCode front-door scan.
func OpenCodeResumeAlias(arguments []string) ([]string, error) {
	return resumeAlias(arguments, openCodeInteractiveValueOptions)
}

// KiloResumeAlias uses Kilo's fixed native value options over the same scan.
func KiloResumeAlias(arguments []string) ([]string, error) {
	return resumeAlias(arguments, kiloInteractiveValueOptions)
}

// resumeAlias rewrites the wrapper's --resume <id> and --resume=<id> into the
// native two-token "-s <id>" form in place, before the literal "--". Wrapper
// value flags (-g/--group, -n/--peer-name) and native value options keep their
// following token untouched, so a value of "--resume" is never rewritten. The
// ID is forwarded verbatim; native OpenCode remains the selection authority.
func resumeAlias(arguments, valueOptions []string) ([]string, error) {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return append(result, arguments[index:]...), nil
		}
		switch {
		case argument == "--resume":
			if index+1 == len(arguments) || arguments[index+1] == "--" || strings.TrimSpace(arguments[index+1]) == "" {
				return nil, errors.New("--resume requires a native session ID")
			}
			result, index = append(result, "-s", arguments[index+1]), index+1
			continue
		case strings.HasPrefix(argument, "--resume="):
			_, id, _ := strings.Cut(argument, "=")
			if strings.TrimSpace(id) == "" {
				return nil, errors.New("--resume requires a native session ID")
			}
			result = append(result, "-s", id)
			continue
		}
		result = append(result, argument)
		wrapperValue := argument == "-g" || argument == "--group" || argument == "-n" || argument == "--peer-name"
		if (wrapperValue || slices.Contains(valueOptions, argument)) && index+1 < len(arguments) {
			index++
			result = append(result, arguments[index])
		}
	}
	return result, nil
}
