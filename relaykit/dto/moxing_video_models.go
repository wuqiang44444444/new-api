package dto

// MoxingVideoModelContract describes the Go reference contract used by
// migration regression fixtures. Runtime validation and public projection read
// the pinned Seedance plugin metadata in plugins/seedance-link/plugin.js.
type MoxingVideoModelContract struct {
	ProviderModel        string
	Resolutions          []string
	SuggestedResolutions []string
	OmitOutputFormat     bool
	// DefaultDurationSeconds is the published northbound default fulfilled
	// southbound when the client omits duration.
	DefaultDurationSeconds int
	MinDurationSeconds     int
	MaxDurationSeconds     int
	// IntelligentDurationSeconds is the frozen billing upper bound for an
	// explicit duration of -1. It estimates budget only and is never sent
	// upstream. 0 means intelligent duration is unsupported.
	IntelligentDurationSeconds int
	// MaxImages, MaxVideos, MaxAudios and MaxTotalMedia are documented
	// per-model reference caps; 0 means the Moxing documentation gives no basis
	// for a local limit and the provider judges the actual media constraints.
	MaxImages     int
	MaxVideos     int
	MaxAudios     int
	MaxTotalMedia int
	// AllowReferenceVideos / AllowReferenceAudios accept the corresponding
	// northbound reference content types.
	AllowReferenceVideos bool
	AllowReferenceAudios bool
	// AllowAudioOnly accepts audio-only reference input. Moxing documents this
	// per model. Examples pairing audio with images do not establish a
	// mandatory pairing rule for other valid northbound inputs.
	AllowAudioOnly bool
	// DefaultGenerateAudio is the documented output-sound default. It is a
	// billing and request default and stays independent of reference audio
	// input.
	DefaultGenerateAudio bool
}

// MoxingVideoModelContracts lists the registered Moxing models in a fixed
// order. The four precise model IDs are the protocol's full model list.
var MoxingVideoModelContracts = []MoxingVideoModelContract{
	{
		// The 0818 model uses official content and defaults output audio to true.
		// Its documentation does not prohibit audio-only reference input.
		ProviderModel:              "doubao-seedance-2-0-260128-0818",
		Resolutions:                []string{"480p", "720p", "1080p", "4k"},
		OmitOutputFormat:           true,
		DefaultDurationSeconds:     5,
		MinDurationSeconds:         4,
		MaxDurationSeconds:         15,
		IntelligentDurationSeconds: 15,
		AllowReferenceVideos:       true,
		AllowReferenceAudios:       true,
		AllowAudioOnly:             true,
		DefaultGenerateAudio:       true,
	},
	{
		ProviderModel:              "doubao-seedance-2-0-fast-260128",
		SuggestedResolutions:       []string{"480p", "720p"},
		OmitOutputFormat:           true,
		DefaultDurationSeconds:     5,
		MinDurationSeconds:         4,
		MaxDurationSeconds:         15,
		IntelligentDurationSeconds: 15,
		AllowReferenceVideos:       true,
		AllowReferenceAudios:       true,
		AllowAudioOnly:             true,
		DefaultGenerateAudio:       true,
	},
	{
		ProviderModel:              "doubao-seedance-2-0-mini-260615",
		SuggestedResolutions:       []string{"480p", "720p"},
		OmitOutputFormat:           true,
		DefaultDurationSeconds:     5,
		MinDurationSeconds:         4,
		MaxDurationSeconds:         15,
		IntelligentDurationSeconds: 15,
		AllowReferenceVideos:       true,
		AllowReferenceAudios:       true,
		AllowAudioOnly:             true,
		DefaultGenerateAudio:       true,
	},
	{
		// 2.5 documents the 30/10/10 per-type caps, the 50-item total, the
		// independent audio reference, seed and the mp4/mov output formats.
		ProviderModel:              "doubao-seedance-2-5-260628",
		Resolutions:                []string{"480p", "720p"},
		DefaultDurationSeconds:     5,
		MinDurationSeconds:         4,
		MaxDurationSeconds:         30,
		IntelligentDurationSeconds: 30,
		MaxImages:                  30,
		MaxVideos:                  10,
		MaxAudios:                  10,
		MaxTotalMedia:              50,
		AllowReferenceVideos:       true,
		AllowReferenceAudios:       true,
		AllowAudioOnly:             true,
		DefaultGenerateAudio:       true,
	},
}

// MoxingVideoModelContractFor returns the registration for one precise Moxing
// provider model. It never infers identity from a customer model name.
func MoxingVideoModelContractFor(providerModel string) (MoxingVideoModelContract, bool) {
	for _, contract := range MoxingVideoModelContracts {
		if contract.ProviderModel == providerModel {
			return contract, true
		}
	}
	return MoxingVideoModelContract{}, false
}
