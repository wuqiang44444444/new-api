package controller

func retiredVideoModel(modelName string) bool {
	return modelName == "sora-2" || modelName == "sora-2-pro"
}

func withoutRetiredVideoModels(modelNames []string) []string {
	filtered := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		if !retiredVideoModel(modelName) {
			filtered = append(filtered, modelName)
		}
	}
	return filtered
}
