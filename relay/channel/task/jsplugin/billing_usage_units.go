package jsplugin

// BillingUsageUnits returns an independent projection of the selected plugin's
// validated schema. It must be captured at submission, never looked up by a
// historical task's model name after the plugin has changed.
func (a *TaskAdaptor) BillingUsageUnits() map[string]string {
	if a.plugin == nil {
		return nil
	}
	units := make(map[string]string, len(a.plugin.Meta.UsageSchema))
	for key, field := range a.plugin.Meta.UsageSchema {
		switch {
		case field.Type == "number":
			units[key] = field.Unit
		case len(field.Enum) > 0:
			units[key] = "enum"
		default:
			units[key] = field.Type
		}
	}
	return units
}
