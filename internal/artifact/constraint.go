package artifact

import "strings"

// Version constraint operators shown by the package form. OpNone means "the
// version as written": the constraint is kept verbatim (for example 3.11.*,
// a bare conda version, or a compound range like >=1.2,<2). OpAny means any
// version and is written as "*".
const (
	OpNone       = "none"
	OpAny        = "any"
	OpGE         = ">="
	OpEQ         = "=="
	OpLE         = "<="
	OpGT         = ">"
	OpLT         = "<"
	OpNE         = "!="
	OpCompatible = "~="
)

// AnyVersion is the constraint pixi uses for "any version".
const AnyVersion = "*"

// ConstraintOperator is one entry of the form's operator picker.
type ConstraintOperator struct {
	Op    string `json:"op"`
	Label string `json:"label"`
}

// ConstraintOperators lists the picker entries in prototype order.
var ConstraintOperators = []ConstraintOperator{
	{OpNone, "the version as written"},
	{OpGE, "at least"},
	{OpEQ, "exactly"},
	{OpLE, "at most"},
	{OpGT, "newer than"},
	{OpLT, "older than"},
	{OpNE, "anything but"},
	{OpCompatible, "compatible with"},
}

// splitOrder lists operators longest first so ">=" wins over ">".
var splitOrder = []string{OpEQ, OpGE, OpLE, OpNE, OpCompatible, OpGT, OpLT}

// SplitConstraint splits a version constraint into the form's operator and
// version. "" and "*" give (OpAny, ""). A single operator followed by a
// plain version gives (op, version). Anything else (bare versions, globs,
// conda's single "=", compound ranges) gives (OpNone, constraint).
func SplitConstraint(constraint string) (op, version string) {
	c := strings.TrimSpace(constraint)
	if c == "" || c == AnyVersion {
		return OpAny, ""
	}
	if strings.ContainsAny(c, ",|") {
		return OpNone, c
	}
	for _, o := range splitOrder {
		if strings.HasPrefix(c, o) {
			v := strings.TrimSpace(c[len(o):])
			if v == "" || strings.ContainsAny(v, "<>=!~ \t") {
				return OpNone, c
			}
			return o, v
		}
	}
	return OpNone, c
}

// JoinConstraint is the inverse of SplitConstraint and mirrors the
// prototype: an empty version means "*", a version that already starts with
// an operator (or OpNone) is used as written, otherwise op+version.
func JoinConstraint(op, version string) string {
	v := strings.TrimSpace(version)
	if v == "" || op == OpAny {
		return AnyVersion
	}
	if op == OpNone || op == "" || strings.ContainsAny(v[:1], "<>=!~") {
		return v
	}
	return op + v
}

// NormalizeConstraint trims a constraint and maps "" to "*".
func NormalizeConstraint(constraint string) string {
	c := strings.TrimSpace(constraint)
	if c == "" {
		return AnyVersion
	}
	return c
}
