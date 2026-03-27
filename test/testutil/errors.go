/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package testutil

import "strings"

func NotFoundErrMeg(b []byte) bool {
	return strings.HasSuffix(strings.TrimRight(string(b), "\r\n"), "not found")
}
