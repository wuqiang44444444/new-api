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

export const meta = {
  apiVersion: 1,
  key: "seedance-link",
  name: "Seedance Link",
  description: {
    en: "Seedance Link southbound protocol adapters",
    zh: "Seedance Link 南向协议适配",
  },
  version: "1.0.2",
  author: { name: "yuan-gateway" },
  seedanceProtocols: ["feicai_videos_v1"],
};

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
    if (isControlChar(code) || code === 0xad || code === 0xfeff ||
        (code >= 0x200b && code <= 0x200f) || (code >= 0x2028 && code <= 0x202e)) {
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
    providerModel: "seedance-2.0-vip-720p-mini-azhw", resolution: "720p", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance2.0-sd2", resolution: "720p", minDuration: 11, maxDuration: 15,
    minImages: 1, maxImages: 9, maxAudios: 0, maxVideos: 0, ratios: SD2_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-720p-fast-azhw", resolution: "720p", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-933-720p-azhw", resolution: "720p", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-720p-azhw", resolution: "720p", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-933-1080p-azhw", resolution: "1080p", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-1080p-azhw", resolution: "1080p", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-933-4k-azhw", resolution: "4k", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-2.0-vip-4k-azhw", resolution: "4k", minDuration: 4, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 0, ratios: STANDARD_RATIOS,
  },
  {
    providerModel: "seedance-933-pro-pi", resolution: "720p", minDuration: 15, maxDuration: 15,
    minImages: 0, maxImages: 9, maxAudios: 3, maxVideos: 3, ratios: STANDARD_RATIOS,
  },
];

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
  if (request.callback_url !== undefined || request.service_tier !== undefined ||
      request.generate_audio !== undefined || request.watermark !== undefined ||
      request.return_last_frame !== undefined || request.execution_expires_after !== undefined ||
      request.draft !== undefined || request.tools !== undefined ||
      request.safety_identifier !== undefined || request.priority !== undefined ||
      request.frames !== undefined || request.seed !== undefined ||
      request.camera_fixed !== undefined || request.output_format !== undefined) {
    throw new Error("request contains fields unsupported by the selected customer model");
  }
}

// ---------------------------------------------------------------------------
// feicai_videos_v1 hooks
// ---------------------------------------------------------------------------

export const seedance = {
  "feicai_videos_v1": {
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
  if (request.duration === null || request.duration === undefined ||
      request.duration < spec.minDuration || request.duration > spec.maxDuration ||
      request.duration > input.limits.maxDurationSeconds) {
    throw new Error(`duration must be between ${spec.minDuration} and ${spec.maxDuration} seconds for the selected customer model`);
  }
  if (request.resolution === null || request.resolution === undefined ||
      trimSpace(request.resolution).toLowerCase() !== spec.resolution) {
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
  } catch (error) {
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
  } catch (error) {
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
