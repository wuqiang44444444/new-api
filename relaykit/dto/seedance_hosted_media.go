package dto

// PlatformHostedAssetMedia is the account library's file format contract.
func PlatformHostedAssetMedia() []PublicAssetSourceMediaTypes {
	return []PublicAssetSourceMediaTypes{
		{MediaType: "image", ContentTypes: []string{"image/jpeg", "image/png", "image/webp", "image/bmp", "image/tiff", "image/gif"}},
		{MediaType: "audio", ContentTypes: []string{"audio/mpeg"}},
		{MediaType: "video", ContentTypes: []string{"video/mp4"}},
	}
}
