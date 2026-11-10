package agent

import _ "embed"

// manual is the agent's page of the manual.
//
//go:embed manual.txt
var manual string

// decisionManual is the decision's page of the manual.
//
//go:embed decision.txt
var decisionManual string
