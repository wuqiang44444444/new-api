package dto

// ImageTokenUsage records reported modality counts for consumption logs.
// A nil count means the upstream did not provide a usable value.
type ImageTokenUsage struct {
	TextInput   *int `json:"text_input,omitempty"`
	ImageInput  *int `json:"image_input,omitempty"`
	TextOutput  *int `json:"text_output,omitempty"`
	ImageOutput *int `json:"image_output,omitempty"`
}
