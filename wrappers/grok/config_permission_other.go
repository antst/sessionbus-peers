// SPDX-License-Identifier: MIT

//go:build !unix

package grok

import "os"

func preserveConfigOwner(_ *os.File, _ os.FileInfo) error { return nil }
