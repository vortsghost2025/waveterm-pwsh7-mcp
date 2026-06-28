// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshutil

var agentAllowedCommands = map[string]struct{}{
	"agentissuetoken":      {},
	"agentgetscrollback":  {},
	"agentsendinput":      {},
	"agentruncommand":     {},
	"agentlistblocks":     {},
	"agentlistterminals":  {},
	"memoryput":             {},
	"memoryget":             {},
	"memorylist":            {},
	"memorydelete":          {},
	"memorysearch":          {},
	"memorydeletemany":      {},
	"memorydeletebyscope":   {},
	"aisendmessage":         {},
}

func isAgentAllowedCommand(command string) bool {
	_, ok := agentAllowedCommands[command]
	return ok
}
