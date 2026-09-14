// Seedance Link southbound protocol extension.
//
// This artifact is the plugin-side implementation of the feicai_videos_v1
// protocol conversion previously hardcoded in
// relay/channel/task/seedance/thirdparty/feicai. It is a byte-faithful port:
// validation rules, wire request shape, and error message strings must match
// the Go implementation exactly (differential tests pin this). The Go host
// keeps URL joining, bearer authentication, same-origin result validation,
// error sanitization, and all billing; credentials never reach this script.
//
// Helpers below replicate Go standard-library semantics where JavaScript
// differs: strings.TrimSpace (unicode.IsSpace set), unicode.IsControl,
// strconv.Quote (%q), utf8 string length, and strict StdEncoding base64
// validity.

// ---------------------------------------------------------------------------
// Go-exact helpers
// ---------------------------------------------------------------------------

// Go strings.TrimSpace trims unicode.IsSpace runes: \t \n \v \f \r space,
// U+0085, U+00A0, and the Unicode Z* separators. JavaScript's trim() differs
// (it removes U+FEFF but not U+0085), so the set is spelled out.
const GO_SPACE = "[\\t\\n\\v\\f\\r \\u0085\\u00A0\\u1680\\u2000-\\u200A\\u2028\\u2029\\u202F\\u205F\\u3000]";

export function trimSpace(value) {
  const pattern = new RegExp("^(?:" + GO_SPACE + ")+|(?:" + GO_SPACE + ")+$", "gu");
  return String(value).replace(pattern, "");
}

// Go unicode.IsControl: the Cc category (C0 and C1 controls).
function isControlChar(code) {
  return (code >= 0x00 && code <= 0x1f) || (code >= 0x7f && code <= 0x9f);
}

function isControlString(value) {
  for (const char of String(value)) {
    if (isControlChar(char.codePointAt(0))) {
      return true;
    }
  }
  return false;
}

// Go len(string) counts UTF-8 bytes, not code points.
function utf8Length(value) {
  let total = 0;
  for (const char of String(value)) {
    const code = char.codePointAt(0);
    if (code <= 0x7f) {
      total += 1;
    } else if (code <= 0x7ff) {
      total += 2;
    } else if (code <= 0xffff) {
      total += 3;
    } else {
      total += 4;
    }
  }
  return total;
}

// Approximates Go strconv.Quote (%q): printable runes stay literal, the C
// escapes and \xHH cover controls, and well-known format runes are escaped.
// Non-printable code points outside these ranges are rare; tests pin the
// practical space.
function quote(value) {
  let out = '"';
  for (const char of String(value)) {
    const code = char.codePointAt(0);
    switch (char) {
      case '"':
        out += '\\"';
        continue;
      case "\\":
        out += "\\\\";
        continue;
      case "\u0007":
        out += "\\a";
        continue;
      case "\b":
        out += "\\b";
        continue;
      case "\f":
        out += "\\f";
        continue;
      case "\n":
        out += "\\n";
        continue;
      case "\r":
        out += "\\r";
        continue;
      case "\t":
        out += "\\t";
        continue;
      case "\v":
        out += "\\v";
        continue;
    }
    if (isControlChar(code) || code === 0xad || code === 0xfeff || (code >= 0x200b && code <= 0x200f) || (code >= 0x2028 && code <= 0x202e)) {
      if (code <= 0xff) {
        out += "\\x" + code.toString(16).padStart(2, "0");
      } else {
        out += "\\u" + code.toString(16).padStart(4, "0");
      }
      continue;
    }
    out += char;
  }
  return out + '"';
}

// Match Go StdEncoding: standard alphabet and padding; CR/LF are ignored.
const STRICT_BASE64 = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;

// ---------------------------------------------------------------------------
// Media URL validation (feicai media_url.go)
// ---------------------------------------------------------------------------

const IMAGE_MIME = new Set(["image/jpeg", "image/png", "image/webp"]);
const MAX_MEDIA_URL_LENGTH = 20 * 1024 * 1024;

// Structural http(s) URL check mirroring the Go net/url decisions that
// matter here: an http/https scheme, a non-empty host, no userinfo, no
// whitespace/control runes, and no invalid percent escapes in the host.
// (The runtime has no WHATWG URL constructor, so this is a scanner, not a
// parser; the differential tests pin the observable behavior.)
function httpURLDetails(value) {
  // eslint-disable-next-line no-control-regex -- Provider IDs and URLs must reject control characters.
  if (/[\x00-\x1f\x7f]/.test(value)) {
    return null; // Go net/url rejects control characters anywhere in the URL
  }
  const match = /^([hH][tT][tT][pP][sS]?):\/\/([^/?#]*)([\s\S]*)$/.exec(value);
  if (match === null) {
    return null;
  }
  const authority = match[2];
  const at = authority.lastIndexOf("@");
  if (at >= 0) {
    return null; // userinfo present, Go reports User != nil
  }
  const host = authority;
  if (host === "") {
    return null;
  }
  // eslint-disable-next-line no-control-regex -- Provider IDs and URLs must reject control characters.
  if (/[\s\x00-\x1f\x7f-\x9f]/.test(host)) {
    return null;
  }
  if (/%(?![0-9a-fA-F]{2})/.test(host)) {
    return null;
  }
  return { secure: match[1].length === 5 };
}

function validateMediaURL(value, mediaType) {
  value = trimSpace(value);
  if (value === "" || value.length > MAX_MEDIA_URL_LENGTH) {
    throw new Error("media URL is empty or too large");
  }
  if (mediaType !== "image" && mediaType !== "audio" && mediaType !== "video") {
    throw new Error("unsupported media type");
  }
  if (value.startsWith("asset://")) {
    if (trimSpace(value.slice("asset://".length)) === "") {
      throw new Error("asset URL is invalid");
    }
    return;
  }
  if (value.startsWith("data:")) {
    const comma = value.indexOf(",");
    if (comma <= "data:".length) {
      throw new Error("invalid data URL");
    }
    // Audio/video representations not explicitly prohibited by the provider
    // stay intact for upstream validation.
    if (mediaType !== "image") return;
    const metadata = value.slice("data:".length, comma).split(";");
    const mime = trimSpace(metadata[0]).toLowerCase();
    if (!IMAGE_MIME.has(mime) || metadata.length !== 2 || metadata[1].toLowerCase() !== "base64") {
      throw new Error("data URL MIME or encoding is not supported");
    }
    if (!STRICT_BASE64.test(value.slice(comma + 1).replace(/[\r\n]/g, ""))) {
      throw new Error("data URL payload is not valid base64");
    }
    return;
  }
  const details = httpURLDetails(value);
  if (details === null) {
    if (mediaType === "image") {
      throw new Error("media URL must be an http(s) URL or an image data URL");
    }
    throw new Error("media URL must be an http(s) URL");
  }
}

// ---------------------------------------------------------------------------
// Model specs (feicai model_spec.go)
// ---------------------------------------------------------------------------

const STANDARD_RATIOS = ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"];
const SD2_RATIOS = ["16:9", "9:16"];

const MODEL_SPECS = [
  {
    providerModel: "seedance-2.0-vip-720p-mini-azhw",
    resolution: "720p",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance2.0-sd2",
    resolution: "720p",
    minDuration: 11,
    maxDuration: 15,
    minImages: 1,
    maxImages: 9,
    maxAudios: 0,
    maxVideos: 0,
    ratios: SD2_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-720p-fast-azhw",
    resolution: "720p",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-933-720p-azhw",
    resolution: "720p",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-720p-azhw",
    resolution: "720p",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-933-1080p-azhw",
    resolution: "1080p",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-1080p-azhw",
    resolution: "1080p",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-933-4k-azhw",
    resolution: "4k",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-4k-azhw",
    resolution: "4k",
    minDuration: 4,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 0,
    ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-933-pro-pi",
    resolution: "720p",
    minDuration: 15,
    maxDuration: 15,
    minImages: 0,
    maxImages: 9,
    maxAudios: 3,
    maxVideos: 3,
    ratios: STANDARD_RATIOS,
  },
];

const MODELARK_RATIOS = ["16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"];
const OFFICIAL_DEFAULT_METADATA = {
  minDuration: 1,
  maxDuration: 60,
  intelligentDuration: true,
  resolutions: ["480p", "720p", "1080p", "4K"],
  ratios: MODELARK_RATIOS,
  allowVideos: true,
  allowAudios: true,
  allowGenerateAudio: true,
  allowWatermark: true,
  allowSeed: true,
  allowCameraFixed: true,
  fullModelArk: true,
};
const OFFICIAL_BASE_METADATA = {
  minDuration: 4,
  maxDuration: 15,
  resolutions: ["480p", "720p"],
  ratios: MODELARK_RATIOS,
  maxImages: 9,
  maxVideos: 3,
  maxAudios: 3,
  allowVideos: true,
  allowAudios: true,
  allowGenerateAudio: true,
  allowWatermark: true,
};
const OFFICIAL_MODEL_METADATA = {};
for (const prefix of ["doubao", "dreamina"]) {
  OFFICIAL_MODEL_METADATA[prefix + "-seedance-2-0-260128"] = { ...OFFICIAL_BASE_METADATA, resolutions: ["480p", "720p", "1080p", "4K"] };
  OFFICIAL_MODEL_METADATA[prefix + "-seedance-2-0-fast-260128"] = OFFICIAL_BASE_METADATA;
  OFFICIAL_MODEL_METADATA[prefix + "-seedance-2-0-mini-260615"] = OFFICIAL_BASE_METADATA;
  OFFICIAL_MODEL_METADATA[prefix + "-seedance-2-5-260628"] = {
    ...OFFICIAL_BASE_METADATA,
    resolutions: ["480p", "720p", "1080p"],
    maxDuration: 30,
    intelligentDuration: true,
    maxImages: 30,
    maxVideos: 10,
    maxAudios: 10,
    allowSeed: true,
    outputFormats: ["mp4", "mov"],
  };
}
const FEICAI_MODEL_METADATA = Object.fromEntries(
  MODEL_SPECS.map((spec) => [
    spec.providerModel,
    {
      minDuration: spec.minDuration,
      maxDuration: spec.maxDuration,
      durationRequired: true,
      resolutionRequired: true,
      ratioRequired: true,
      ratios: spec.ratios,
      resolutions: [spec.resolution],
      minImages: spec.minImages,
      maxImages: spec.maxImages,
      maxVideos: spec.maxVideos,
      maxAudios: spec.maxAudios,
      allowVideos: spec.maxVideos > 0,
      allowAudios: spec.maxAudios > 0,
    },
  ])
);

const TOKENSAVE_MODEL_METADATA = {
  "doubao-seedance-2-0-260128": {
    minDuration: 4,
    maxDuration: 15,
    intelligentDuration: true,
    intelligentDurationSeconds: 15,
    resolutions: ["480p", "720p", "1080p"],
    ratios: MODELARK_RATIOS,
    allowVideos: true,
    allowAudios: true,
    allowAudioOnly: true,
    allowGenerateAudio: true,
    allowWatermark: true,
    allowSeed: true,
    allowCameraFixed: true,
  },
};
const MOXING_MODEL_METADATA = Object.fromEntries(
  ["doubao-seedance-2-0-260128-0818", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628"].map((model) => [
    model,
    {
      minDuration: 4,
      maxDuration: model === "doubao-seedance-2-5-260628" ? 30 : 15,
      intelligentDuration: true,
      intelligentDurationSeconds: model === "doubao-seedance-2-5-260628" ? 30 : 15,
      defaultDuration: 5,
      defaultGenerateAudio: true,
      allowAudioOnly: true,
      freeResolution: true,
      ratios: MODELARK_RATIOS,
      maxImages: model === "doubao-seedance-2-5-260628" ? 30 : 0,
      maxVideos: model === "doubao-seedance-2-5-260628" ? 10 : 0,
      maxAudios: model === "doubao-seedance-2-5-260628" ? 10 : 0,
      maxTotalMedia: model === "doubao-seedance-2-5-260628" ? 50 : 0,
      allowVideos: true,
      allowAudios: true,
      allowGenerateAudio: true,
      allowWatermark: true,
      allowSeed: true,
      allowCameraFixed: true,
      fullModelArk: true,
    },
  ])
);

// Moxing model pages distinguish exhaustive supported values from suggestions.
// Suggestions must never become a request rejection rule.
Object.assign(MOXING_MODEL_METADATA["doubao-seedance-2-0-260128-0818"], {
  freeResolution: false,
  resolutions: ["480p", "720p", "1080p", "4k"],
  omitOutputFormat: true,
});
Object.assign(MOXING_MODEL_METADATA["doubao-seedance-2-5-260628"], {
  freeResolution: false,
  resolutions: ["480p", "720p"],
});
for (const model of ["doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615"]) {
  Object.assign(MOXING_MODEL_METADATA[model], {
    suggestedResolutions: ["480p", "720p"],
    omitOutputFormat: true,
  });
}

const CMCC_MODEL_METADATA = {
  "doubao-seedance-2.0": {
    minDuration: 4,
    maxDuration: 15,
    resolutions: ["480p", "720p", "1080p"],
    ratios: ["16:9", "9:16", "1:1"],
    allowVideos: true,
    allowAudios: true,
    allowGenerateAudio: true,
    allowWatermark: true,
  },
};
const FUNCLOUD_MODEL_METADATA = Object.fromEntries(
  ["seedance-2-0", "seedance-2-0-fast", "seedance-2-0-mini", "seedance-2-5"].map((model) => [
    model,
    {
      minDuration: 4,
      maxDuration: model === "seedance-2-5" ? 30 : 15,
      intelligentDuration: model === "seedance-2-5",
      intelligentDurationSeconds: model === "seedance-2-5" ? 30 : 15,
      defaultDuration: 5,
      defaultGenerateAudio: true,
      allowAudioOnly: true,
      resolutions: model === "seedance-2-5" ? ["480p", "720p", "1080p"] : ["480p", "720p"],
      ratios: MODELARK_RATIOS,
      maxImages: 30,
      maxVideos: 10,
      maxAudios: 10,
      allowVideos: true,
      allowAudios: true,
      allowGenerateAudio: true,
      allowWatermark: true,
      allowSeed: true,
      allowCameraFixed: true,
      outputFormats: ["mp4", "mov"],
      allowReturnLastFrame: true,
      allowPriority: true,
      deleteVideo: false,
    },
  ])
);
const SYNLINK_MODEL_METADATA = Object.fromEntries(
  ["doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628"].map((model) => [
    model,
    {
      // Synlink delegates model limits to ModelArk. Common request examples
      // are not an exhaustive capability list for every model.
      ...OFFICIAL_MODEL_METADATA[model],
      intelligentDuration: false,
      allowSeed: false,
      outputFormats: [],
      defaultDuration: 5,
      publishGenerateAudioDefault: true,
      defaultGenerateAudio: false,
      resolutions: OFFICIAL_MODEL_METADATA[model].resolutions.map((value) => value.toLowerCase()),
      allowAudioOnly: model === "doubao-seedance-2-5-260628",
      maxTotalMedia: model === "doubao-seedance-2-5-260628" ? 50 : 12,
      ratios: MODELARK_RATIOS,
      allowVideos: true,
      allowAudios: true,
      allowGenerateAudio: true,
      allowWatermark: true,
      allowReturnLastFrame: true,
      deleteVideo: false,
    },
  ])
);
export const meta = {
  apiVersion: 3,
  key: "seedance-link",
  name: "Seedance Link",
  description: {
    en: "Seedance Link southbound protocol adapters",
    zh: "Seedance Link 南向协议适配",
  },
  version: "1.3.3",
  author: { name: "yuan-gateway" },
  seedanceProtocols: [
    "funcloud_modelark_v3",
    "synlink_video_v1",
    "feicai_videos_v1",
    "modelark_v3_volcengine",
    "modelark_v3_byteplus",
    "ark_media_v1",
    "tokensave_media_task_v1",
    "moxing_modelark_media_v1",
    "modelark_v3_cmcc",
  ],
  channelConfiguration: {
    videos: [
      {
        protocol: "funcloud_modelark_v3",
        label: "FunCloud ModelArk V3",
        models: Object.keys(FUNCLOUD_MODEL_METADATA),
        modelMetadata: FUNCLOUD_MODEL_METADATA,
        assetProtocols: ["funcloud_material", "funcloud_material_hosted", "none"],
        defaultAssetProtocol: "funcloud_material",
      },
      {
        protocol: "synlink_video_v1",
        label: "Synlink Video V1",
        models: Object.keys(SYNLINK_MODEL_METADATA),
        modelMetadata: SYNLINK_MODEL_METADATA,
        assetProtocols: ["funcloud_material_hosted", "none"],
        defaultAssetProtocol: "funcloud_material_hosted",
      },
      {
        protocol: "modelark_v3_cmcc",
        label: "CMCC Mobile Cloud ModelArk V3",
        models: Object.keys(CMCC_MODEL_METADATA),
        modelMetadata: CMCC_MODEL_METADATA,
        assetProtocols: ["cmcc_aicc_assets_v2", "none"],
        defaultAssetProtocol: "cmcc_aicc_assets_v2",
      },
      {
        protocol: "tokensave_media_task_v1",
        label: "TokenSave Media Task V1",
        models: Object.keys(TOKENSAVE_MODEL_METADATA),
        modelMetadata: TOKENSAVE_MODEL_METADATA,
        assetProtocols: ["tokensave_assets_v1", "none"],
        defaultAssetProtocol: "tokensave_assets_v1",
      },
      {
        protocol: "moxing_modelark_media_v1",
        label: "Moxing ModelArk Media Task V1",
        models: Object.keys(MOXING_MODEL_METADATA),
        modelMetadata: MOXING_MODEL_METADATA,
        assetProtocols: ["moxing_volc_assets_v1", "none"],
        defaultAssetProtocol: "moxing_volc_assets_v1",
      },
      {
        protocol: "ark_media_v1",
        label: "Ark Media V1",
        models: [],
        modelPolicy: "configured",
        defaultModelMetadata: { ...OFFICIAL_DEFAULT_METADATA, resolutions: ["480p", "720p", "1080p"] },
        assetProtocols: ["ark_assets_v1", "none"],
        defaultAssetProtocol: "ark_assets_v1",
      },
      {
        protocol: "modelark_v3_byteplus",
        label: "BytePlus ModelArk V3",
        models: Object.keys(OFFICIAL_MODEL_METADATA),
        modelPolicy: "configured",
        modelMetadata: OFFICIAL_MODEL_METADATA,
        defaultModelMetadata: OFFICIAL_DEFAULT_METADATA,
        assetProtocols: ["byteplus_assets_action_v2024_01_01", "none"],
        defaultAssetProtocol: "byteplus_assets_action_v2024_01_01",
      },
      {
        protocol: "modelark_v3_volcengine",
        label: "Volcengine ModelArk V3",
        models: Object.keys(OFFICIAL_MODEL_METADATA),
        modelPolicy: "configured",
        modelMetadata: OFFICIAL_MODEL_METADATA,
        defaultModelMetadata: OFFICIAL_DEFAULT_METADATA,
        assetProtocols: ["volcengine_assets_action_v2024_01_01", "none"],
        defaultAssetProtocol: "volcengine_assets_action_v2024_01_01",
      },
      {
        protocol: "feicai_videos_v1",
        label: "Feicai Videos V1 (URL Only, No Asset Library)",
        models: MODEL_SPECS.map((spec) => spec.providerModel),
        modelMetadata: FEICAI_MODEL_METADATA,
        assetProtocols: ["none"],
        defaultAssetProtocol: "none",
      },
    ],
    assets: [
      {
        protocol: "funcloud_material",
        label: "FunCloud Material Library",
        credential: "channel",
        groupPolicy: "default_fallback",
        connectivity: true,
        defaultURLTTLSeconds: 3600,
        operations: ["create_asset", "get_asset", "delete_asset", "create_asset_group", "get_asset_group"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
        ],
      },
      {
        protocol: "funcloud_material_hosted",
        label: "Platform Hosted Image Library",
        credential: "none",
        groupPolicy: "hosted",
        operations: ["create_asset", "get_asset", "delete_asset", "create_asset_group", "get_asset_group"],
        media: [{ kind: "general", mediaType: "image" }],
      },
      {
        protocol: "cmcc_aicc_assets_v2",
        label: "CMCC AICC Assets V2",
        credential: "asset_key_pair",
        groupPolicy: "default_fallback",
        connectivity: true,
        defaultURLTTLSeconds: 3600,
        operations: ["create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group", "get_asset_group_verification"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
          { kind: "real_person", mediaType: "image" },
          { kind: "real_person", mediaType: "video" },
          { kind: "real_person", mediaType: "audio" },
        ],
      },
      {
        protocol: "tokensave_assets_v1",
        deleteNotFoundIsSuccess: true,
        label: "TokenSave Asset Library V1",
        groupPolicy: "default_fallback",
        credential: "channel",
        defaultURLTTLSeconds: 3600,
        operations: ["create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
        ],
      },
      {
        protocol: "moxing_volc_assets_v1",
        deleteNotFoundIsSuccess: true,
        connectivity: true,
        label: "Moxing Volcengine Asset Library V1",
        groupPolicy: "default_fallback",
        credential: "channel",
        project: { required: true, fixed: "default" },
        defaultURLTTLSeconds: 3600,
        operations: ["create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group", "get_asset_group_verification"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
          { kind: "real_person", mediaType: "image" },
        ],
      },
      {
        protocol: "ark_assets_v1",
        deleteNotFoundIsSuccess: true,
        connectivity: true,
        label: "Ark Assets V1",
        groupPolicy: "default_fallback",
        credential: "channel",
        defaultURLTTLSeconds: 3600,
        operations: ["create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group", "get_asset_group_verification"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
          { kind: "real_person", mediaType: "image" },
        ],
      },
      {
        protocol: "byteplus_assets_action_v2024_01_01",
        label: "BytePlus Official Assets",
        groupPolicy: "default_fallback",
        groupSearch: true,
        connectivity: true,
        operations: ["create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group", "get_asset_group_verification"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
          { kind: "real_person", mediaType: "image" },
        ],
        credential: "asset_key_pair",
        project: { required: true },
        region: { required: true, format: "region_id" },
        defaultURLTTLSeconds: 3600,
      },
      {
        protocol: "volcengine_assets_action_v2024_01_01",
        label: "Volcengine Official Assets",
        groupPolicy: "default_fallback",
        groupSearch: true,
        connectivity: true,
        operations: ["create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group", "get_asset_group_verification"],
        media: [
          { kind: "general", mediaType: "image" },
          { kind: "general", mediaType: "video" },
          { kind: "general", mediaType: "audio" },
          { kind: "real_person", mediaType: "image" },
        ],
        credential: "asset_key_pair",
        project: { required: true },
        region: { required: true, fixed: "cn-beijing" },
        defaultURLTTLSeconds: 3600,
      },
      { protocol: "none", label: "No Asset Protocol", groupPolicy: "none", credential: "none" },
    ],
  },
};

function resolveModelSpec(providerModel) {
  providerModel = trimSpace(providerModel);
  for (const spec of MODEL_SPECS) {
    if (spec.providerModel !== providerModel) {
      continue;
    }
    if (spec.ratios.length === 0) {
      return null;
    }
    return spec;
  }
  return null;
}

function rejectUnsupportedFields(request) {
  if (
    request.callback_url !== undefined ||
    request.service_tier !== undefined ||
    request.generate_audio !== undefined ||
    request.watermark !== undefined ||
    request.return_last_frame !== undefined ||
    request.execution_expires_after !== undefined ||
    request.draft !== undefined ||
    request.tools !== undefined ||
    request.safety_identifier !== undefined ||
    request.priority !== undefined ||
    request.frames !== undefined ||
    request.seed !== undefined ||
    request.camera_fixed !== undefined ||
    request.output_format !== undefined
  ) {
    throw new Error("request contains fields unsupported by the selected customer model");
  }
}

// ---------------------------------------------------------------------------
// feicai_videos_v1 hooks
// ---------------------------------------------------------------------------

export const seedance = {
  funcloud_modelark_v3: { buildCreate: buildFunCloudVideoCreate, parseCreateResponse: parseFunCloudVideoCreate, parseTaskObservation: parseFunCloudVideoTask },
  synlink_video_v1: { buildCreate: buildSynlinkVideoCreate, parseCreateResponse: parseSynlinkVideoCreate, parseTaskObservation: parseSynlinkVideoTask },
  modelark_v3_cmcc: { buildCreate: buildCMCCVideoCreate, parseCreateResponse: parseOfficialVideoCreateResponse, parseTaskObservation: parseOfficialVideoTask },
  tokensave_media_task_v1: { buildCreate: buildTokenSaveVideoCreate, parseCreateResponse: parseRelayVideoCreate, parseTaskObservation: parseRelayVideoTask },
  moxing_modelark_media_v1: { buildCreate: buildMoxingVideoCreate, parseCreateResponse: parseRelayVideoCreate, parseTaskObservation: parseRelayVideoTask },
  ark_media_v1: { buildCreate: buildArkVideoCreate, parseCreateResponse: parseArkVideoCreate, parseTaskObservation: parseArkVideoTask },
  modelark_v3_byteplus: {
    buildCreate: buildOfficialVideoCreate,
    parseCreateResponse: parseOfficialVideoCreateResponse,
    parseTaskObservation: parseOfficialVideoTask,
  },
  modelark_v3_volcengine: {
    buildCreate: buildOfficialVideoCreate,
    parseCreateResponse: parseOfficialVideoCreateResponse,
    parseTaskObservation: parseOfficialVideoTask,
  },
  feicai_videos_v1: {
    buildCreate(input) {
      return buildFeicaiCreate(input);
    },
    parseCreateResponse(input) {
      return parseFeicaiCreateResponse(input);
    },
    parseTaskObservation(input) {
      return parseFeicaiTaskObservation(input);
    },
  },
};

function buildFeicaiCreate(input) {
  const request = input.request;
  const providerModel = trimSpace(input.providerModel);
  const spec = resolveModelSpec(providerModel);
  if (spec === null) {
    throw new Error("the selected customer model is not supported by its configured video adapter");
  }
  if (
    request.duration === null ||
    request.duration === undefined ||
    request.duration < spec.minDuration ||
    request.duration > spec.maxDuration ||
    request.duration > input.limits.maxDurationSeconds
  ) {
    throw new Error(`duration must be between ${spec.minDuration} and ${spec.maxDuration} seconds for the selected customer model`);
  }
  if (request.resolution === null || request.resolution === undefined || trimSpace(request.resolution).toLowerCase() !== spec.resolution) {
    throw new Error(`resolution must be ${quote(spec.resolution)} for the selected customer model`);
  }
  if (request.ratio === null || request.ratio === undefined) {
    throw new Error("aspect ratio is required for the selected customer model");
  }
  const ratio = trimSpace(request.ratio);
  if (!spec.ratios.includes(ratio)) {
    throw new Error(`aspect ratio ${quote(ratio)} is not supported by the selected customer model`);
  }
  rejectUnsupportedFields(request);

  const resolved = { model: providerModel, prompt: "", duration: request.duration, ratio, images: [], audios: [], videos: [] };
  for (const item of request.content || []) {
    switch (trimSpace(item.type)) {
      case "text": {
        let text = "";
        if (item.text !== null && item.text !== undefined) {
          text = trimSpace(item.text);
        }
        if (text !== "") {
          if (resolved.prompt !== "") {
            resolved.prompt += "\n";
          }
          resolved.prompt += text;
        }
        break;
      }
      case "image_url": {
        if (item.image_url == null || item.role == null || trimSpace(item.role) !== "reference_image") {
          throw new Error("image_url requires role=reference_image");
        }
        const mediaURL = trimSpace(item.image_url.url);
        try {
          validateMediaURL(mediaURL, "image");
        } catch (error) {
          throw new Error(`invalid image input: ${error.message}`);
        }
        resolved.images.push(mediaURL);
        break;
      }
      case "audio_url": {
        if (item.audio_url == null || item.role == null || trimSpace(item.role) !== "reference_audio") {
          throw new Error("audio_url requires role=reference_audio");
        }
        const mediaURL = trimSpace(item.audio_url.url);
        try {
          validateMediaURL(mediaURL, "audio");
        } catch (error) {
          throw new Error(`invalid audio input: ${error.message}`);
        }
        resolved.audios.push(mediaURL);
        break;
      }
      case "video_url": {
        if (item.video_url == null || item.role == null || trimSpace(item.role) !== "reference_video") {
          throw new Error("video_url requires role=reference_video");
        }
        const mediaURL = trimSpace(item.video_url.url);
        try {
          validateMediaURL(mediaURL, "video");
        } catch (error) {
          throw new Error(`invalid video input: ${error.message}`);
        }
        resolved.videos.push(mediaURL);
        break;
      }
      default:
        throw new Error(`unsupported content type ${quote(item.type)}`);
    }
  }
  if (resolved.prompt === "") {
    throw new Error("prompt is required for the selected customer model");
  }
  if (resolved.images.length < spec.minImages || resolved.images.length > spec.maxImages) {
    throw new Error(`image count must be between ${spec.minImages} and ${spec.maxImages} for the selected customer model`);
  }
  if (resolved.audios.length > spec.maxAudios) {
    throw new Error(`audio count must not exceed ${spec.maxAudios} for the selected customer model`);
  }
  if (resolved.videos.length > spec.maxVideos) {
    throw new Error(`video count must not exceed ${spec.maxVideos} for the selected customer model`);
  }

  const body = {
    model: resolved.model,
    prompt: resolved.prompt,
    duration: resolved.duration,
    ratio: resolved.ratio,
  };
  if (resolved.images.length > 0) {
    body.images = resolved.images;
  }
  if (resolved.audios.length > 0) {
    body.audios = resolved.audios;
  }
  if (resolved.videos.length > 0) {
    body.videos = resolved.videos;
  }
  return {
    body,
    probe: {
      resolution: spec.resolution,
      ratio: resolved.ratio,
      size_multiplier: 1.0,
      billing_mode: "per-second",
    },
  };
}

function parseFeicaiCreateResponse(input) {
  let response;
  try {
    response = JSON.parse(input.body);
  } catch {
    throw new Error("upstream create response is invalid JSON");
  }
  if (response === null || typeof response !== "object" || Array.isArray(response)) {
    throw new Error("upstream create response is invalid JSON");
  }
  if (response.id !== undefined && response.id !== null && typeof response.id !== "string") {
    throw new Error("upstream create response is invalid JSON");
  }
  const id = trimSpace(response.id === undefined || response.id === null ? "" : response.id);
  if (id === "") {
    throw new Error("upstream create response has no id");
  }
  if (utf8Length(id) > 191 || isControlString(id)) {
    throw new Error("upstream create response has an invalid id");
  }
  return { id };
}

function parseFeicaiTaskObservation(input) {
  let response;
  try {
    response = JSON.parse(input.body);
  } catch {
    return { violation: "invalid JSON task response" };
  }
  if (response === null || typeof response !== "object" || Array.isArray(response)) {
    return { violation: "invalid JSON task response" };
  }
  if (response.id !== undefined && response.id !== null && typeof response.id !== "string") {
    return { violation: "invalid JSON task response" };
  }
  if (response.status !== undefined && response.status !== null && typeof response.status !== "string") {
    return { violation: "invalid JSON task response" };
  }
  if (response.video_url !== undefined && response.video_url !== null && typeof response.video_url !== "string") {
    return { violation: "invalid JSON task response" };
  }
  const errorValue = response.error === undefined || response.error === null ? {} : response.error;
  if (typeof errorValue !== "object" || Array.isArray(errorValue)) {
    return { violation: "invalid JSON task response" };
  }
  for (const field of ["code", "message"]) {
    if (errorValue[field] !== undefined && errorValue[field] !== null && typeof errorValue[field] !== "string") {
      return { violation: "invalid JSON task response" };
    }
  }
  const id = trimSpace(response.id === undefined || response.id === null ? "" : response.id);
  if (id === "" || id !== input.taskId) {
    return { violation: "task id mismatch" };
  }
  const status = response.status === undefined || response.status === null ? "" : response.status;
  switch (status) {
    case "queued":
      return { id, status: "queued" };
    case "processing":
    case "in_progress":
      return { id, status: "running" };
    case "completed":
      return {
        id,
        status: "succeeded",
        videoUrl: response.video_url === undefined || response.video_url === null ? "" : response.video_url,
      };
    case "failed":
      return {
        id,
        status: "failed",
        error: {
          code: errorValue.code === undefined || errorValue.code === null ? "" : errorValue.code,
          message: errorValue.message === undefined || errorValue.message === null ? "" : errorValue.message,
        },
      };
    default:
      return { violation: "unsupported task status" };
  }
}

// Official Action wire conversion. The host supplies only non-secret operation
// arguments. HTTP destination, credential injection and V4 signing stay in Go.
export const seedanceAssets = {
  funcloud_material: { buildRequest: buildFunCloudAssetRequest, parseResponse: parseFunCloudAssetResponse },
  cmcc_aicc_assets_v2: { buildRequest: buildCMCCAssetRequest, parseResponse: parseCMCCAssetResponse },
  tokensave_assets_v1: { buildRequest: buildTokenSaveAssetRequest, parseResponse: parseTokenSaveAssetResponse },
  moxing_volc_assets_v1: { buildRequest: buildMoxingAssetRequest, parseResponse: parseMoxingAssetResponse },
  ark_assets_v1: { buildRequest: buildArkAssetRequest, parseResponse: parseArkAssetResponse },
  byteplus_assets_action_v2024_01_01: { buildRequest: buildOfficialAssetRequest, parseResponse: parseOfficialAssetResponse },
  volcengine_assets_action_v2024_01_01: {
    buildRequest: buildOfficialAssetRequest,
    parseResponse: parseOfficialAssetResponse,
  },
};

function buildOfficialAssetRequest(input) {
  const request = input.request;
  const project = input.project;
  let action;
  let body = { ProjectName: project };
  switch (input.operation) {
    case "create_asset":
      action = "CreateAsset";
      Object.assign(body, {
        GroupId: request.GroupResourceID,
        URL: request.URL,
        AssetType: trimSpace(request.MediaType).toLowerCase()[0].toUpperCase() + trimSpace(request.MediaType).toLowerCase().slice(1),
        Name: request.Name,
      });
      break;
    case "get_asset":
    case "delete_asset":
      action = input.operation === "get_asset" ? "GetAsset" : "DeleteAsset";
      body.Id = request.id;
      break;
    case "update_asset":
      action = "UpdateAsset";
      Object.assign(body, { Id: request.id, Name: request.name });
      break;
    case "create_group":
      action = "CreateAssetGroup";
      Object.assign(body, { Name: request.Name, Description: request.Description, GroupType: trimSpace(request.GroupType) || "AIGC" });
      break;
    case "get_group":
      action = "GetAssetGroup";
      body.Id = request.id;
      break;
    case "list_assets":
    case "list_groups": {
      action = input.operation === "list_assets" ? "ListAssets" : "ListAssetGroups";
      const groupType = trimSpace(request.GroupType);
      if (!groupType) throw new Error("official Action " + action + " requires GroupType");
      const filter = { GroupType: groupType };
      if (request.GroupIDs && request.GroupIDs.length) filter.GroupIds = request.GroupIDs;
      if (input.operation === "list_assets" && request.Statuses && request.Statuses.length) filter.Statuses = request.Statuses;
      if (trimSpace(request.Name)) filter.Name = trimSpace(request.Name);
      Object.assign(body, { Filter: filter, PageNumber: request.Page > 0 ? request.Page : 1, PageSize: request.PageSize > 0 ? request.PageSize : 100 });
      break;
    }
    case "create_verification":
      action = "CreateVisualValidateSession";
      body.CallbackURL = request.RedirectURL;
      break;
    case "get_verification":
    case "get_verification_session":
      action = "GetVisualValidateResult";
      body.BytedToken = request.id;
      break;
    default:
      throw new Error("unsupported_asset_operation");
  }
  return { action, body };
}

function officialAssetResult(item) {
  const id = strictOptionalString(item, "Id");
  const originalStatus = strictOptionalString(item, "Status");
  let status = trimSpace(originalStatus).toLowerCase();
  const result = {
    ResourceID: id,
    BusinessID: id,
    ErrorCode: strictOptionalString(item, "ErrorCode"),
    ErrorMessage: strictOptionalString(item, "ErrorMessage"),
  };
  if (status === "active") {
    result.ReferenceType = "asset_uri_id";
    result.ReferenceValue = id;
  } else if (status === "" || status === "processing") {
    status = "processing";
  } else if (status !== "failed") {
    status = originalStatus.toLowerCase();
  }
  result.Status = status;
  return result;
}

function strictOptionalString(object, key) {
  if (object == null) return "";
  if (typeof object !== "object" || Array.isArray(object)) throw new Error("invalid asset response");
  const value = object[key];
  if (value === undefined || value === null) return "";
  if (typeof value !== "string") throw new Error("invalid asset response field type");
  return value;
}

function officialGroupResult(item) {
  const id = strictOptionalString(item, "Id");
  return {
    ResourceID: id,
    BusinessID: id,
    Name: strictOptionalString(item, "Name"),
    Status: trimSpace(strictOptionalString(item, "Status")).toLowerCase() || "active",
  };
}

function parseOfficialAssetResponse(input) {
  const raw = JSON.parse(input.body);
  if (raw != null && (typeof raw !== "object" || Array.isArray(raw))) throw new Error("invalid asset response");
  const metadataError = raw && raw.ResponseMetadata && raw.ResponseMetadata.Error;
  if (input.status < 200 || input.status >= 300) {
    let code = (metadataError && metadataError.Code) || "";
    if (typeof code !== "string") code = "";
    code = trimSpace(code);
    if (code.length > 128 || !/^[a-zA-Z0-9._-]*$/.test(code)) code = "";
    if (!code && metadataError && Number.isSafeInteger(metadataError.CodeN) && metadataError.CodeN !== 0) code = String(metadataError.CodeN);
    return { httpError: { code } };
  }
  if (metadataError && Object.keys(metadataError).length) {
    const number = typeof metadataError.CodeN === "number" ? Math.trunc(metadataError.CodeN) : 50000;
    return { applicationError: { code: Number.isSafeInteger(number) ? number : 50000 } };
  }
  const result = raw && Object.prototype.hasOwnProperty.call(raw, "Result") ? raw.Result : raw;
  if (result != null && (typeof result !== "object" || Array.isArray(result))) throw new Error("invalid asset response");
  const item = result || {};
  switch (input.operation) {
    case "delete_asset":
      return { result: {} };
    case "create_asset":
    case "get_asset":
    case "update_asset": {
      const asset = officialAssetResult(item);
      if (input.operation === "update_asset" && !asset.ResourceID) {
        asset.ResourceID = input.request.id;
        asset.BusinessID = input.request.id;
        if (asset.ReferenceType) asset.ReferenceValue = input.request.id;
      }
      return { result: asset };
    }
    case "create_group":
    case "get_group":
      return { result: officialGroupResult(item) };
    case "list_assets":
    case "list_groups": {
      if (item.Items != null && !Array.isArray(item.Items)) throw new Error("invalid asset list response");
      const total = item.TotalCount == null ? 0 : item.TotalCount;
      if (!Number.isSafeInteger(total)) throw new Error("invalid asset list total");
      return { result: { Items: (item.Items || []).map(input.operation === "list_assets" ? officialAssetResult : officialGroupResult), Total: total } };
    }
    case "create_verification":
      return {
        result: {
          SessionID: strictOptionalString(item, "BytedToken"),
          H5URL: strictOptionalString(item, "H5Link"),
          Status: "verifying",
          ExpiresAt: input.now + 1800,
        },
      };
    case "get_verification":
    case "get_verification_session": {
      const id = strictOptionalString(item, "GroupId");
      return { result: { GroupID: id, Status: id ? "active" : "verifying" } };
    }
    default:
      throw new Error("unsupported_asset_operation");
  }
}

function buildOfficialVideoCreate(input) {
  return {
    body: { ...input.request, model: input.providerModel },
    probe: {},
    createPath: "/api/v3/contents/generations/tasks",
    queryPath: "/api/v3/contents/generations/tasks/{task_id}",
  };
}
function parseOfficialVideoCreateResponse(input) {
  const response = JSON.parse(input.body);
  return { id: strictOptionalString(response, "id") };
}
// Keep JSON number spelling for integer usage fields: 1.0 and 1e0 were not
// accepted as Go int64 values and must not become new billing evidence.
function parseOfficialVideoTask(input) {
  let response;
  try {
    response = JSON.parse(input.body);
  } catch {
    return { violation: "invalid ModelArk task response" };
  }
  if (!response || response.id !== input.taskId || !response.id) return { violation: "ModelArk task id mismatch" };
  if (response.status != null && typeof response.status !== "string") return { violation: "invalid ModelArk task status" };
  if (!Object.prototype.hasOwnProperty.call(response, "status")) return { violation: "invalid ModelArk task status" };
  // v3 observation contract: identity and status validation only. Usage is
  // derived by the host from the raw upstream bytes before this hook runs, so
  // the strict official ModelArk integer semantics never cross the JS number
  // boundary and charging facts never depend on artifact behavior.
  return { body: input.body };
}

function buildArkAssetRequest(input) {
  const r = input.request,
    root = "/v1/ark/assets",
    id = r.escapedID;
  switch (input.operation) {
    case "create_asset":
      return { method: "POST", path: root, body: { GroupId: r.GroupResourceID, URL: r.URL, AssetType: assetMediaType(r.MediaType), Name: r.Name } };
    case "get_asset":
      return { method: "GET", path: root + "/" + id, body: null };
    case "update_asset":
      return { method: "POST", path: root + "/" + id + "/update", body: { Name: r.name } };
    case "delete_asset":
      return { method: "POST", path: root + "/" + id + "/delete", body: null };
    case "create_group":
      return { method: "POST", path: root + "/groups", body: { Name: r.Name, Description: r.Description, GroupType: "AIGC" } };
    case "get_group":
      return { method: "GET", path: root + "/groups/" + id, body: null };
    case "list_assets":
      return { method: "POST", path: root + "/list", body: { PageNumber: 1, PageSize: 1 } };
    case "create_verification":
      return { method: "POST", path: root + "/visual-validate/session", body: { client_redirect_url: r.RedirectURL, project_name: r.ProjectName } };
    case "get_verification_session":
      return { method: "GET", path: root + "/visual-validate/sessions/" + id, body: null };
    case "get_verification":
      return { method: "GET", path: root + "/visual-validate/result/" + id, body: null };
    default:
      throw new Error("unsupported_asset_operation");
  }
}
function assetMediaType(value) {
  const media = trimSpace(value).toLowerCase();
  return media ? media[0].toUpperCase() + media.slice(1) : "";
}
function parseArkAssetResponse(input) {
  if (input.operation === "delete_asset" || input.operation === "list_assets") return { result: {} };
  const item = JSON.parse(input.body);
  if (input.operation.includes("verification"))
    return {
      result: {
        SessionID: strictOptionalString(item, "session_id"),
        GroupID: strictOptionalString(item, "group_id"),
        H5URL: strictOptionalString(item, "h5_link"),
        Status: strictOptionalString(item, "status"),
        ExpiresAt: strictOptionalInteger(item, "expires_at"),
      },
    };
  let id = strictOptionalString(item, "Id");
  if (!id && input.operation === "update_asset") id = input.request.id;
  let status = strictOptionalString(item, "Status").toLowerCase();
  const code = strictOptionalString(item, "ErrorCode");
  if (input.operation.endsWith("group")) return { result: { ResourceID: id, BusinessID: id, Status: code ? "failed" : status || "active" } };
  const result = { ResourceID: id, BusinessID: id, ErrorCode: code, ErrorMessage: strictOptionalString(item, "ErrorMessage") };
  if (code) result.Status = "failed";
  else if (status === "active") Object.assign(result, { Status: "active", ReferenceType: "asset_uri_id", ReferenceValue: id });
  else result.Status = status === "failed" || status === "pending" ? status : "processing";
  return { result };
}
function strictOptionalInteger(object, key) {
  const value = object && object[key];
  if (value == null) return 0;
  if (!Number.isSafeInteger(value)) throw new Error("invalid integer response field");
  return value;
}
function buildArkVideoCreate(input) {
  return {
    body: { ...input.request, model: input.providerModel },
    probe: {},
    createPath: "/v1/ark/media/generations",
    queryPath: "/v1/ark/media/tasks/{task_id}",
  };
}
function responseObject(body) {
  const result = JSON.parse(body);
  if (result === null) return {};
  if (typeof result !== "object" || Array.isArray(result)) throw new Error("invalid task response");
  return result;
}
function firstResponseString(object, ...keys) {
  if (!object || typeof object !== "object" || Array.isArray(object)) return "";
  for (const key of keys) if (typeof object[key] === "string" && trimSpace(object[key])) return object[key];
  return "";
}
function responseData(root) {
  return root.data && typeof root.data === "object" && !Array.isArray(root.data) ? root.data : root;
}
function parseArkVideoCreate(input) {
  const root = responseObject(input.body),
    data = responseData(root);
  const id = firstResponseString(data, "id", "task_id") || firstResponseString(root, "id", "task_id");
  if (!id) throw new Error("upstream create response has no task id");
  return { id };
}
function parseArkVideoTask(input) {
  const root = responseObject(input.body),
    data = responseData(root);
  let status = trimSpace(firstResponseString(data, "status", "state")).toLowerCase();
  if (status === "success" || status === "completed") status = "succeeded";
  if (["failure", "error", "cancelled", "canceled", "expired"].includes(status)) status = "failed";
  if (!["pending", "queued", "processing", "running", "succeeded", "failed"].includes(status)) return { violation: "unsupported task status" };
  const result = { id: firstResponseString(data, "id", "task_id"), status };
  const video = firstResponseString(data.content, "video_url") || firstResponseString(data.output, "video_url") || firstResponseString(data, "video_url");
  if (status === "succeeded" && !video) return { violation: "succeeded task has no result URL" };
  if (video) result.content = { video_url: video };
  const model = firstResponseString(data, "model");
  if (model) result.model = model;
  if (status === "succeeded") Object.assign(result, usageScanRoot({ usage: data.usage }));
  const message = firstResponseString(data.error, "message") || firstResponseString(data, "message");
  if (message) result.error = { message };
  return { body: JSON.stringify(result) };
}
// v3 observation contract: the artifact only locates the Provider usage
// subtree that it used to scan. Bucketing, invalid-field blocking, completion
// derivation and evidence collection are host-owned (thirdparty usage
// normalization), so quota semantics cannot drift between Go and JS.
function usageScanRoot(root) {
  return root && typeof root === "object" ? { usage_scan_root: root } : {};
}

function validateMediaModelRequest(request, spec, fullModelArk) {
  if (!spec) throw new Error("the selected customer model is not supported by its configured video adapter");
  if (!fullModelArk) {
    if ((request.return_last_frame != null && !spec.allowReturnLastFrame) || (request.priority != null && !spec.allowPriority))
      throw new Error("request contains a parameter unsupported by the selected customer model");
  }
  if (!fullModelArk)
    for (const key of ["callback_url", "service_tier", "execution_expires_after", "draft", "tools", "safety_identifier", "frames"])
      if (request[key] != null) throw new Error("request contains a parameter unsupported by the selected customer model");
  const duration = request.duration == null ? 5 : request.duration;
  if (duration === -1 && !spec.intelligentDuration) throw new Error("intelligent duration is not supported by the selected customer model");
  if (duration !== -1 && (duration < spec.minDuration || duration > spec.maxDuration))
    throw new Error("duration must be between " + spec.minDuration + " and " + spec.maxDuration + " for the selected customer model");
  const resolution = request.resolution == null ? "720p" : trimSpace(request.resolution);
  if (!resolution) throw new Error("resolution must not be empty");
  if (spec.resolutions && spec.resolutions.length && !spec.resolutions.includes(resolution))
    throw new Error('resolution "' + resolution + '" is not supported by the selected customer model');
  if (request.ratio != null) {
    const ratio = trimSpace(request.ratio);
    if (!ratio) throw new Error("ratio must not be empty");
    if (!MODELARK_RATIOS.includes(ratio)) throw new Error('ratio "' + ratio + '" is not supported by the selected customer model');
  }
  if (request.output_format != null) {
    if (spec.omitOutputFormat) throw new Error("output_format is not supported by the selected customer model");
    const format = trimSpace(request.output_format);
    if (!format) throw new Error("output_format must not be empty");
    if (!(fullModelArk ? ["mp4", "mov"] : spec.outputFormats || []).includes(format))
      throw new Error('output_format "' + format + '" is not supported by the selected customer model');
  }
  const count = { image_url: 0, video_url: 0, audio_url: 0 };
  for (const item of request.content || []) if (item.type in count) count[item.type]++;
  if (count.video_url && !spec.allowVideos) throw new Error("video_url content is not supported by the selected customer model");
  if (count.audio_url && !spec.allowAudios) throw new Error("audio_url content is not supported by the selected customer model");
  for (const [type, key, label] of [
    ["image_url", "maxImages", "images"],
    ["video_url", "maxVideos", "videos"],
    ["audio_url", "maxAudios", "audios"],
  ])
    if (spec[key] > 0 && count[type] > spec[key])
      throw new Error("at most " + spec[key] + " reference " + label + " are supported by the selected customer model");
  if (spec.maxTotalMedia > 0 && Object.values(count).reduce((a, b) => a + b, 0) > spec.maxTotalMedia)
    throw new Error("at most " + spec.maxTotalMedia + " reference media items are supported by the selected customer model");
  if (!spec.allowAudioOnly && count.audio_url && !count.image_url && !count.video_url)
    throw new Error("audio-only input is not supported by the selected customer model");
}
function buildMoxingVideoCreate(input) {
  const spec = MOXING_MODEL_METADATA[input.providerModel];
  validateMediaModelRequest(input.request, spec, true);
  const body = { ...input.request, model: input.providerModel };
  if (body.generate_audio == null) body.generate_audio = spec.defaultGenerateAudio;
  if (body.duration == null && body.frames == null) body.duration = spec.defaultDuration;
  return { body, probe: {}, createPath: "/v1/media/generations", queryPath: "/v1/media/tasks/{task_id}" };
}
function buildTokenSaveVideoCreate(input) {
  validateMediaModelRequest(input.request, TOKENSAVE_MODEL_METADATA[input.providerModel], false);
  const r = input.request,
    body = { model: trimSpace(input.providerModel), capability: "video_generation", input_mode: "text", control_mode: "none" };
  for (const [from, to] of [
    ["duration", "duration_seconds"],
    ["generate_audio", "with_audio"],
    ["resolution", "resolution"],
    ["ratio", "aspect_ratio"],
    ["seed", "seed"],
    ["camera_fixed", "camera_fixed"],
    ["watermark", "watermark"],
  ])
    if (r[from] != null && r[from] !== "") body[to] = r[from];
  const first = [],
    images = [],
    videos = [],
    audios = [],
    prompts = [];
  for (const item of r.content || []) {
    const type = trimSpace(item.type).toLowerCase();
    if (!type || type === "text") {
      if (trimSpace(item.text)) prompts.push(trimSpace(item.text));
      continue;
    }
    if (!["image_url", "video_url", "audio_url"].includes(type)) throw new Error('unsupported content type "' + item.type + '"');
    const value = item[type] && item[type].url;
    if (!trimSpace(value)) throw new Error(type + ".url is required");
    if (type === "image_url") {
      const role = trimSpace(item.role).toLowerCase();
      if (!role || role === "first_frame") first.push(trimSpace(value));
      else if (role === "last_frame") {
        if (body.end_image) throw new Error("only one last_frame image is supported");
        body.end_image = trimSpace(value);
      } else if (role === "reference_image") images.push(trimSpace(value));
      else throw new Error('unsupported image role "' + item.role + '"');
    } else if (type === "video_url") videos.push(value);
    else audios.push(value);
  }
  if (first.length > 1) throw new Error("multiple first-frame images require role=reference_image");
  if (first.length) body.image = first[0];
  if (prompts.length) body.prompt = prompts.join("\n");
  if (images.length) body.reference_images = images;
  if (videos.length) body.reference_videos = videos;
  if (audios.length) body.reference_audios = audios;
  if (images.length + videos.length + audios.length) {
    body.input_mode = "multi_image";
    body.control_mode = "reference";
  } else if (body.end_image) {
    body.input_mode = "single_image";
    body.control_mode = "end_frame";
  } else if (body.image) body.input_mode = "single_image";
  if (!body.prompt && !body.image && !body.end_image && !images.length && !videos.length && !audios.length)
    throw new Error("prompt or media content is required");
  return { body, probe: {}, createPath: "/v1/media/generations", queryPath: "/v1/media/tasks/{task_id}" };
}
function trustedRelayID(value) {
  const id = trimSpace(value);
  if (!id) throw new Error("upstream response has no task id");
  // eslint-disable-next-line no-control-regex -- Provider IDs and URLs must reject control characters.
  if (id.length > 191 || /[\u0000-\u001f\u007f-\u009f]/.test(id)) throw new Error("upstream response has an invalid task id");
  return id;
}
function parseRelayVideoCreate(input) {
  const root = responseObject(input.body),
    data = responseData(root);
  return { id: trustedRelayID(firstResponseString(data, "task_id", "id") || firstResponseString(root, "task_id", "id")) };
}
function videoResultURL(result, validURLs) {
  const valid = (value) => (validURLs ? validURLs[value] === true : /^https:\/\/[^\s/@]+(?:[/?#]|$)/i.test(value));
  if (typeof result === "string") {
    const value = trimSpace(result);
    if (valid(value)) return value;
    try {
      result = JSON.parse(value);
    } catch {
      return "";
    }
  }
  if (!result || typeof result !== "object") return "";
  for (const value of [result.primary_url, result.url, ...(Array.isArray(result.urls) ? result.urls : [])])
    if (typeof value === "string" && valid(trimSpace(value))) return trimSpace(value);
  return "";
}
function parseRelayVideoTask(input) {
  const root = responseObject(input.body),
    data = responseData(root),
    id = trustedRelayID(firstResponseString(data, "task_id", "id"));
  if (id !== trimSpace(input.taskId)) return { violation: "task id mismatch" };
  let status = trimSpace(firstResponseString(data, "status", "state")).toLowerCase();
  if (["pending", "submitted"].includes(status)) status = "queued";
  else if (status === "processing") status = "running";
  else if (["success", "completed"].includes(status)) status = "succeeded";
  else if (["failure", "error", "cancelled", "canceled"].includes(status)) status = "failed";
  if (!["queued", "running", "succeeded", "failed"].includes(status)) return { violation: "unsupported task status" };
  const result = { id, status },
    video = videoResultURL(data.result, input.validResultURLs),
    model = firstResponseString(data, "model");
  if (model) result.model = model;
  if (status === "succeeded" && !video) return { violation: "succeeded task has no result URL" };
  if (video) result.content = { video_url: video };
  if (status === "succeeded") Object.assign(result, usageScanRoot(root));
  if (status === "failed")
    result.error = {
      message:
        firstResponseString(data, "error_message") ||
        firstResponseString(data.error, "message") ||
        firstResponseString(data, "message") ||
        "upstream task failed",
    };
  return { body: JSON.stringify(result) };
}

function buildTokenSaveAssetRequest(input) {
  const r = input.request,
    root = "/v1/asset",
    id = r.escapedID;
  switch (input.operation) {
    case "create_asset":
      return { method: "POST", path: root + "/create", body: { groupId: r.GroupResourceID, URL: r.URL, AssetType: assetMediaType(r.MediaType), Name: r.Name } };
    case "get_asset":
      return { method: "POST", path: root + "/detail/" + id, body: null };
    case "update_asset":
      return { method: "POST", path: root + "/" + id, body: { Name: r.name } };
    case "delete_asset":
      return { method: "DELETE", path: root + "/" + id, body: null };
    case "create_group":
      return { method: "POST", path: root + "/group/create", body: { Name: r.Name, Description: r.Description, GroupType: "AIGC" } };
    case "get_group":
      return { method: "POST", path: root + "/group/detail/" + id, body: null };
    default:
      throw new Error("unsupported_asset_operation");
  }
}
function parseTokenSaveAssetResponse(input) {
  const root = responseObject(input.body),
    r = root.result || {};
  if (root.error != null) return { applicationError: { code: strictOptionalInteger(root.error, "code") } };
  if (input.operation === "delete_asset") return { result: {} };
  const group = input.operation.endsWith("group"),
    nested = (group ? r.group : r.asset) || {};
  if (input.operation === "update_asset" && !strictOptionalString(nested, "id")) nested.id = input.request.id;
  const direct = !strictOptionalString(nested, "id") && strictOptionalString(r, "id"),
    item = direct ? r : nested;
  const id = strictOptionalString(item, "id"),
    status = strictOptionalInteger(item, "status"),
    requestId = strictOptionalString(root, "requestId");
  if (group)
    return {
      result: {
        ResourceID: id,
        BusinessID: strictOptionalString(item, "groupId"),
        Status: status === 1 ? "active" : status === 2 ? "failed" : direct ? "active" : "processing",
        RequestID: requestId,
      },
    };
  const vendorStatus = strictOptionalString(item, "vendorStatus").toLowerCase(),
    assetId = strictOptionalString(item, "assetId");
  const result = { ResourceID: id, BusinessID: assetId, ErrorMessage: trimSpace(strictOptionalString(item, "errorMsg")), RequestID: requestId };
  if (status === 2 || vendorStatus === "failed") Object.assign(result, { Status: "failed", ErrorCode: "upstream_asset_failed" });
  else if (status === 1 && vendorStatus === "active") {
    Object.assign(result, { Status: "active", ReferenceType: "asset_uri_id", ReferenceValue: trimSpace(assetId) || trimSpace(id) });
    if (!result.ReferenceValue)
      Object.assign(result, {
        Status: "failed",
        ErrorCode: "upstream_asset_failed",
        ErrorMessage: "Moxing JoyCreator returned Active without an asset id (request " + requestId + ")",
      });
  } else result.Status = "processing";
  return { result };
}
function buildMoxingAssetRequest(input) {
  const r = input.request,
    root = "/v1/volc/assets",
    id = r.escapedID;
  switch (input.operation) {
    case "create_asset":
      return {
        method: "POST",
        path: root,
        body: { GroupId: r.GroupResourceID, URL: r.URL, Name: r.Name, AssetType: assetMediaType(r.MediaType), ProjectName: "default" },
      };
    case "get_asset":
      return { method: "GET", path: root + "/" + id + "?ProjectName=default", body: null };
    case "update_asset":
      return { method: "POST", path: root + "/" + id + "/update", body: { Name: r.name, ProjectName: "default" } };
    case "delete_asset":
      return { method: "POST", path: root + "/" + id + "/delete", body: { ProjectName: "default" } };
    case "create_group":
      return { method: "POST", path: root + "/groups", body: { Name: r.Name, Description: r.Description, GroupType: "AIGC" } };
    case "get_group":
      return { method: "GET", path: root + "/groups/" + id + "?ProjectName=default", body: null };
    case "list_assets":
      return { method: "POST", path: root + "/list", body: { PageNumber: 1, PageSize: 1, ProjectName: "default" } };
    case "create_verification":
      return { method: "POST", path: root + "/visual-validate/sessions", body: { ProjectName: "default" } };
    case "get_verification_session":
      return { method: "GET", path: root + "/visual-validate/sessions/" + id, body: null };
    case "get_verification":
      return { method: "GET", path: root + "/visual-validate/results/" + id, body: null };
    default:
      throw new Error("unsupported_asset_operation");
  }
}
function moxingErrorCode(error) {
  const value = error && error.Code;
  if (value == null) return "";
  const code = trimSpace(String(value));
  return code === "0" ? "" : code;
}
function parseMoxingAssetResponse(input) {
  const root = responseObject(input.body),
    item = root.Result || {},
    error = moxingErrorCode(root.Error);
  if (error) {
    const raw = root.Error.Code;
    let code = typeof raw === "number" ? Math.trunc(raw) : /^[+-]?\d+$/.test(trimSpace(raw)) ? Number(trimSpace(raw)) : 500;
    return { applicationError: { code: Number.isSafeInteger(code) ? code : 500 } };
  }
  if (input.operation === "delete_asset" || input.operation === "list_assets") return { result: {} };
  if (input.operation.includes("verification"))
    return {
      result: {
        SessionID: strictOptionalString(item, "SessionId"),
        GroupID: strictOptionalString(item, "GroupId"),
        H5URL: strictOptionalString(item, "H5Link"),
        Status: strictOptionalString(item, "Status"),
        ExpiresAt: strictOptionalInteger(item, "ExpiresAt"),
      },
    };
  let id = strictOptionalString(item, "Id");
  if (!id && input.operation === "update_asset") id = input.request.id;
  const status = trimSpace(strictOptionalString(item, "Status")).toLowerCase(),
    requestId = strictOptionalString(root, "RequestId");
  if (input.operation.endsWith("group"))
    return { result: { ResourceID: id, BusinessID: strictOptionalString(item, "GroupId"), Status: status || "active", RequestID: requestId } };
  const code = trimSpace(strictOptionalString(item, "ErrorCode")) || moxingErrorCode(item.Error),
    message = trimSpace(strictOptionalString(item, "ErrorMessage")) || trimSpace(strictOptionalString(item.Error, "Message"));
  const result = { ResourceID: id, BusinessID: strictOptionalString(item, "AssetId"), ErrorCode: code, ErrorMessage: message, RequestID: requestId };
  if (status === "active") Object.assign(result, { Status: "active", ReferenceType: "asset_uri_id", ReferenceValue: id });
  else result.Status = status === "failed" ? "failed" : "processing";
  if (code) result.Status = "failed";
  return { result };
}

function buildCMCCVideoCreate(input) {
  const r = input.northRequest || input.request,
    spec = CMCC_MODEL_METADATA[input.providerModel];
  if (!spec) throw new Error("the selected customer model is not supported by its configured video adapter");
  for (const field of [
    "callback_url",
    "output_format",
    "service_tier",
    "return_last_frame",
    "execution_expires_after",
    "draft",
    "tools",
    "safety_identifier",
    "priority",
    "frames",
    "seed",
    "camera_fixed",
  ])
    if (r[field] != null) throw new Error("request contains a parameter unsupported by the selected customer model");
  const duration = r.duration == null ? 5 : r.duration;
  if (duration < 4 || duration > 15) throw new Error("duration must be between 4 and 15 for the selected customer model");
  const resolution = r.resolution == null ? "720p" : trimSpace(r.resolution);
  if (!resolution) throw new Error("resolution must not be empty");
  if (!spec.resolutions.includes(resolution)) throw new Error('resolution "' + resolution + '" is not supported by the selected customer model');
  if (r.ratio != null) {
    const ratio = trimSpace(r.ratio);
    if (!ratio) throw new Error("ratio must not be empty");
    if (!spec.ratios.includes(ratio)) throw new Error('ratio "' + ratio + '" is not supported by the selected customer model');
  }
  return buildOfficialVideoCreate(input);
}
function buildCMCCAssetRequest(input) {
  const r = input.request,
    id = r.escapedID;
  switch (input.operation) {
    case "create_asset":
      return {
        method: "POST",
        path: "/asset",
        body: { groupId: r.GroupResourceID, assetName: r.Name, assetUrl: r.URL, assetType: assetMediaType(r.MediaType) },
      };
    case "get_asset":
      return { method: "GET", path: "/asset/" + id, body: null };
    case "update_asset":
      return { method: "PUT", path: "/asset/" + id, body: { assetName: r.name } };
    case "delete_asset":
      return { method: "DELETE", path: "/asset/" + id, body: null };
    case "list_assets":
      return { method: "POST", path: "/asset/query", body: { pageNo: 1, pageSize: 1, groupType: "AIGC" } };
    case "create_group":
      return { method: "POST", path: "/asset-group", body: { groupType: "AIGC", groupName: r.Name, description: r.Description } };
    case "get_group":
      return { method: "GET", path: "/asset-group/" + id, body: null };
    case "create_verification":
      if (trimSpace(r.RedirectURL)) return { unsupported: true };
      return { method: "POST", path: "/real-person-auth/sessions", body: null };
    case "get_verification":
    case "get_verification_session":
      return { method: "POST", path: "/real-person-auth/asset-group/by-byted-token", body: { bytedToken: r.id } };
    default:
      return { unsupported: true };
  }
}
function parseCMCCAssetResponse(input) {
  const root = responseObject(input.body),
    state = strictOptionalString(root, "state"),
    code = trimSpace(strictOptionalString(root, "errorCode"));
  if (input.status < 200 || input.status >= 300) {
    const message = strictOptionalString(root, "errorMessage"),
      id = input.request.id;
    const single = typeof id === "string" && id !== "" && !id.includes("/");
    const missing =
      state === "ERROR" &&
      single &&
      ((input.operation === "get_asset" && input.status === 500 && code === "C500999" && message === "NotFound.asset_id") ||
        (input.operation === "delete_asset" && input.status === 400 && code === "C400999" && message === "素材不存在或无权限访问"));
    return { httpError: { notFound: missing } };
  }
  if (trimSpace(state).toLowerCase() !== "ok") return { stringError: { code, definitive: true, notFound: /NOT_FOUND|NOTFOUND/.test(code.toUpperCase()) } };
  if (root.body == null) return { stringError: { code: "missing_body" } };
  const item = root.body;
  switch (input.operation) {
    case "delete_asset":
      if (typeof item !== "boolean") throw new Error("invalid deletion result");
      return item ? { result: {} } : { stringError: { code: "delete_rejected", definitive: true } };
    case "create_asset":
      if (typeof item !== "string") throw new Error("invalid asset id");
      return { result: { ResourceID: item, BusinessID: item, Status: "processing" } };
    case "get_asset":
    case "update_asset": {
      let id = strictOptionalString(item, "assetId");
      if (!id && input.operation === "update_asset") id = input.request.id;
      const status = strictOptionalString(item, "status"),
        result = { ResourceID: id, BusinessID: id, Status: trimSpace(status).toLowerCase(), ErrorMessage: strictOptionalString(item, "errorMessage") };
      if (status.toLowerCase() === "active") Object.assign(result, { ReferenceType: "asset_uri_id", ReferenceValue: id });
      if (status.toLowerCase() === "failed") result.ErrorCode = "provider_failed";
      return { result };
    }
    case "create_group":
    case "get_group": {
      const id = strictOptionalString(item, "groupId");
      return { result: { ResourceID: id, BusinessID: id, Status: "active" } };
    }
    case "list_assets":
      return { result: {} };
    case "create_verification": {
      const expires = strictOptionalInteger(item, "expiresIn");
      if (expires > 253402300799 - input.now) throw new Error("invalid verification expiry");
      return {
        result: {
          SessionID: strictOptionalString(item, "bytedToken"),
          H5URL: strictOptionalString(item, "h5Link"),
          Status: "verifying",
          ExpiresAt: expires > 0 ? input.now + expires : 0,
        },
      };
    }
    case "get_verification":
    case "get_verification_session":
      if (typeof item !== "string") throw new Error("invalid verification result");
      return { result: { GroupID: item, Status: trimSpace(item) ? "active" : "verifying" } };
    default:
      return { unsupported: true };
  }
}

function buildFunCloudVideoCreate(input) {
  const spec = FUNCLOUD_MODEL_METADATA[input.providerModel];
  validateMediaModelRequest(input.northRequest || input.request, spec, false);
  const body = { ...input.request, model: input.providerModel };
  if (body.duration == null) body.duration = spec.defaultDuration;
  if (!body.resolution) body.resolution = "720p";
  if (body.generate_audio == null) body.generate_audio = spec.defaultGenerateAudio;
  body.real_person_mode = true;
  if ((body.content || []).some((item) => item.type === "video_url")) body.omni_reference_task_type = "reference";
  return { body, probe: {}, createPath: "/api/v3/contents/generations/tasks", queryPath: "/api/v3/contents/generations/tasks/{task_id}" };
}
function parseFunCloudVideoCreate(input) {
  const root = responseObject(input.body);
  if (root.error != null) throw new Error("create response contains an error");
  return { id: trustedRelayID(firstResponseString(root, "id")) };
}
function parseFunCloudVideoTask(input) {
  const root = responseObject(input.body),
    id = trustedRelayID(firstResponseString(root, "id"));
  if (id !== input.taskId) return { violation: "task id mismatch" };
  let status = firstResponseString(root, "status");
  if (status === "submitted") status = "queued";
  if (!["queued", "running", "succeeded", "failed"].includes(status) || root.status === "queued") return { violation: "unsupported task status" };
  const result = { id, status };
  for (const key of ["model", "created_at", "updated_at", "duration", "resolution", "ratio", "seed", "generate_audio", "frames", "framespersecond"])
    if (key in root) result[key] = root[key];
  if (status === "succeeded") {
    const video = firstResponseString(root.content, "video_url");
    if (!video) return { violation: "invalid video result URL" };
    result.content = { video_url: video };
    const last = firstResponseString(root.content, "last_frame_url");
    if (last) result.content.last_frame_url = last;
    Object.assign(result, usageScanRoot(root));
  }
  if (status === "failed")
    result.error = { code: firstResponseString(root.error, "code"), message: firstResponseString(root.error, "message") || "upstream task failed" };
  return { body: JSON.stringify(result) };
}
function buildSynlinkVideoCreate(input) {
  const spec = SYNLINK_MODEL_METADATA[input.providerModel],
    r = input.northRequest || input.request;
  if (!spec) throw new Error("the selected model is not registered for this video protocol");
  for (const field of [
    "callback_url",
    "service_tier",
    "execution_expires_after",
    "draft",
    "tools",
    "safety_identifier",
    "priority",
    "frames",
    "seed",
    "camera_fixed",
    "output_format",
  ])
    if (r[field] != null) throw new Error("request contains a parameter not published for this video protocol");
  validateMediaModelRequest(r, spec, false);
  const body = { ...input.request, model: input.providerModel };
  if (body.duration == null) body.duration = spec.defaultDuration;
  if (!body.resolution) body.resolution = "720p";
  if (body.generate_audio == null) body.generate_audio = spec.defaultGenerateAudio;
  return { body, probe: {}, createPath: "/v1/video/generate", queryPath: "/v1/video/tasks/{task_id}" };
}
function parseSynlinkVideoCreate(input) {
  const root = responseObject(input.body),
    task = root.task || {},
    id = trustedRelayID(firstResponseString(task, "id"));
  if (root.error != null || task.error != null || firstResponseString(task, "status") !== "pending") throw new Error("untrusted Synlink create response");
  return { id };
}
function parseSynlinkVideoTask(input) {
  const root = responseObject(input.body),
    task = root.task || {},
    id = trustedRelayID(firstResponseString(task, "id"));
  if (id !== input.taskId || root.error != null || task.error != null) return { violation: "untrusted Synlink task identity" };
  const result = { id };
  if (Number.isInteger(task.duration_seconds) && task.duration_seconds >= 1 && task.duration_seconds <= 60) result.duration = task.duration_seconds;
  for (const [from, to] of [
    ["created_at", "created_at"],
    ["completed_at", "updated_at"],
  ]) {
    const value = task[from];
    if (typeof value === "number" && Number.isInteger(value) && value >= 0 && value <= 253402300799) result[to] = value;
    else if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)) {
      const seconds = Math.floor(Date.parse(value) / 1000);
      if (Number.isFinite(seconds) && seconds >= 0) result[to] = seconds;
    }
  }
  const status = firstResponseString(task, "status");
  if (status === "pending") result.status = "queued";
  else if (status === "processing") result.status = "running";
  else if (status === "completed") {
    if (!Array.isArray(task.outputs) || task.outputs.length !== 1 || typeof task.outputs[0] !== "string")
      return { violation: "Synlink completion requires one video output" };
    result.status = "succeeded";
    result.content = { video_url: task.outputs[0] };
    Object.assign(result, usageScanRoot({ task: { usage: task.usage } }));
  } else return { violation: "unverified Synlink task status" };
  return { body: JSON.stringify(result) };
}

function buildFunCloudAssetRequest(input) {
  const r = input.request,
    root = "/api/v2/open/material",
    page = r.page || 1;
  switch (input.operation) {
    case "create_asset":
      return {
        method: "POST",
        path: root + "/virtual/upload",
        body: { multipart: { fileField: "file", fields: { groupId: r.GroupResourceID, materialName: r.Name } } },
      };
    case "get_asset":
      if (!trimSpace(r.id)) throw new Error("FunCloud material id is required");
      return { method: "GET", path: root + "/list?page=" + page + "&pageSize=100&materialCategory=2", body: null };
    case "delete_asset":
      return { method: "POST", path: root + "/delete?materialId=" + r.queryID, body: null };
    case "create_group":
      return { method: "POST", path: root + "/group/create", body: { groupName: r.Name, description: r.Description } };
    case "get_group":
      if (!trimSpace(r.id)) throw new Error("FunCloud material group id is required");
      return { method: "GET", path: root + "/group/list?page=" + page + "&pageSize=100", body: null };
    case "list_assets":
      return { method: "GET", path: root + "/list?page=1&pageSize=1", body: null };
    default:
      return { unsupported: true };
  }
}
function funCloudList(data) {
  if (data == null) return [];
  if (Array.isArray(data)) return data;
  if (typeof data !== "object") throw new Error("invalid material list");
  const collections = [data.list, data.items, data.records].filter((value) => value != null);
  if (collections.length !== 1 || !Array.isArray(collections[0])) throw new Error("material list has no unique collection");
  return collections[0];
}
function funCloudMaterialResult(item) {
  const id = trimSpace(strictOptionalString(item, "materialId")),
    assetURL = trimSpace(strictOptionalString(item, "assetUrl"));
  if (!id) throw new Error("material response has no material id");
  const state = trimSpace(strictOptionalString(item, "assetStatus")).toLowerCase(),
    result = { ResourceID: id, BusinessID: id, Status: state === "active" || state === "failed" ? state : "processing" };
  if (assetURL) {
    if (!assetURL.startsWith("asset://")) throw new Error("invalid asset URL");
    const providerID = trimSpace(assetURL.slice(8));
    // eslint-disable-next-line no-control-regex -- Provider IDs and URLs must reject control characters.
    if (!providerID || /[\\/?#\s\u0000-\u001f\u007f-\u009f]/.test(providerID)) throw new Error("invalid asset URL");
    result.ReferenceType = "asset_uri_id";
    result.ReferenceValue = providerID;
  }
  if (item.isAsset != null && typeof item.isAsset !== "boolean") throw new Error("invalid asset flag");
  if (!state && item.isAsset === true && result.ReferenceValue) result.Status = "active";
  if (result.Status === "active" && !result.ReferenceValue) throw new Error("active material has no asset URL");
  return result;
}
function parseFunCloudAssetResponse(input) {
  const root = responseObject(input.body),
    code = strictOptionalInteger(root, "code");
  if (input.operation === "delete_asset" && root.code == null) throw new Error("material delete response has no code");
  if (input.operation === "delete_asset" && code === 90003) return { notFound: true };
  if (code !== 0)
    return {
      applicationError: {
        code,
        definitive: input.operation !== "create_asset" && input.operation !== "delete_asset" && [10002, 10005, 10006, 30003].includes(code),
      },
    };
  if (input.operation === "delete_asset" || input.operation === "list_assets") return { result: {} };
  if (input.operation === "create_asset") return { result: funCloudMaterialResult(root.data) };
  if (input.operation === "create_group") {
    const id = trimSpace(strictOptionalString(root.data, "groupId"));
    if (!id) throw new Error("invalid material group response");
    return { result: { ResourceID: id, BusinessID: id, Status: "active" } };
  }
  const items = funCloudList(root.data),
    id = trimSpace(input.request.id),
    matches = [];
  for (const item of items) {
    const candidate = trimSpace(strictOptionalString(item, input.operation === "get_group" ? "groupId" : "materialId"));
    if (candidate !== id) continue;
    matches.push(input.operation === "get_group" ? { ResourceID: id, BusinessID: id, Status: "active" } : funCloudMaterialResult(item));
  }
  return { result: { Items: matches, Count: items.length } };
}
