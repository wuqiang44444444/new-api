package common

// Image consumption logs persist these message keys with their parameters.
// Translations may change independently. Keep the stored keys stable so older
// logs can still supply size and quality for report diagnostics.
const (
	ImageSizeLogMessage    = "Size {{size}}"
	ImageQualityLogMessage = "Quality {{quality}}"
)
