package billingexpr

// RequiresUsage reports dependencies on provider usage, using the compiler's
// registered numeric variables rather than a second token-variable list.
func RequiresUsage(expression string) bool {
	for name := range UsedVars(expression) {
		if _, numeric := compileEnvPrototypeV1[name].(float64); numeric || name == "u" {
			return true
		}
	}
	return false
}
