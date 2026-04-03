/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import "strings"

func NotFoundErrMeg(b []byte) bool {
	return strings.HasSuffix(strings.TrimRight(string(b), "\r\n"), "not found")
}
