package billingexpr

import "fmt"

// UnknownIdentifierError reports the first identifier referenced by an
// expression that the runtime environment cannot bind. Settlement paths use
// this to fail closed: the lenient map-based compile environment turns
// unknown identifiers into silent zero lookups, which would otherwise let a
// broken frozen expression settle a task at zero cost.
func UnknownIdentifier(exprStr string) error {
	for name := range UsedVars(exprStr) {
		if _, bound := compileEnvPrototypeV1[name]; !bound {
			return fmt.Errorf("expression references unknown variable %q", name)
		}
	}
	return nil
}
