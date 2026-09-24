// MiniMax Link standard-video southbound extension (JD Cloud task API).
//
// This artifact owns the jdcloud_video_task_v1 conversion: the ModelArk V3
// northbound contract is transformed into the JD Cloud task submit/query
// shapes, and upstream observations are normalized back. The host keeps URL
// joining, bearer authentication, task identity, status/domain validation,
// billing and all persistence; credentials never reach this script.
//
// H3 open set (single source for runtime validation and public projection):
// text plus reference media (9 images, 3 videos, 3 audios), or first/last frames. The
// platform always enables prompt optimization; there is no client switch.
// Official H3 ranges and the separately verified JD combinations are recorded
// in docs/80-dev/2026-09-24-MiniMaxH3多模态真实调用验证.md. Documented ranges
// do not imply that every combination has passed live gateway acceptance.

const MODEL = "MiniMax-H3";

const DEFAULT_DURATION = 6;
const MIN_DURATION = 4;
const MAX_DURATION = 15;

// Northbound enum spelling; the wire spelling is converted by buildCreate.
const RESOLUTIONS = ["768p", "2k"];
const MAX_PROMPT_LENGTH = 7000;
const MAX_IMAGES = 9;
const MAX_AUDIOS = 3;
const MAX_VIDEOS = 3;
const ALLOW_FRAME_IMAGES = true;

const RATIOS = ["16:9", "21:9", "4:3", "1:1", "3:4", "9:16", "adaptive"];
const DEFAULT_RATIO = RATIOS[0];

// Fixed southbound policy; not client-configurable.
const PROMPT_OPTIMIZER = true;
const WATERMARK = false;

// Northbound conditional fields that this contract does not offer.
// Explicit occurrences are rejected as unsupported instead of being dropped,
// clamped or reinterpreted.
const UNSUPPORTED_FIELDS = [
  "callback_url",
  "service_tier",
  "generate_audio",
  "return_last_frame",
  "execution_expires_after",
  "draft",
  "tools",
  "safety_identifier",
  "priority",
  "output_format",
  "seed",
  "camera_fixed",
  "frames",
];

// Registered create acceptance state. The upstream query status enum is NOT
// a create-response enum; only evidence-backed states may be added here.
const ACCEPTED_CREATE_STATUS = "pending";

function isMissing(value) {
  return value === undefined || value === null;
}

// ---------------------------------------------------------------------------
// buildCreate
// ---------------------------------------------------------------------------

function buildCreate(input) {
  const request = input.request;
  const providerModel = String(input.providerModel === undefined || input.providerModel === null ? "" : input.providerModel);
  if (providerModel !== MODEL) {
    throw new Error("the selected customer model is not supported by its configured video adapter");
  }

  for (const field of UNSUPPORTED_FIELDS) {
    if (!isMissing(request[field])) {
      throw new Error("request contains fields unsupported by the MiniMax contract");
    }
  }

  const content = request.content;
  if (!Array.isArray(content) || content.length < 1 || content.length > 1 + MAX_IMAGES + MAX_VIDEOS + MAX_AUDIOS) {
    throw new Error("content requires one text node, at most 9 reference images, 3 reference videos and 3 reference audios");
  }
  let texts = 0, images = 0, videos = 0, audios = 0, firstFrames = 0, lastFrames = 0;
  for (const node of content) {
    if (node === null || typeof node !== "object" || Array.isArray(node)) {
      throw new Error("content contains an invalid node");
    }
    if (node.type === "text") {
      texts++;
      if (typeof node.text !== "string" || node.text.trim() === "" || Array.from(node.text).length > MAX_PROMPT_LENGTH ||
          Object.keys(node).some(key => !["type", "text"].includes(key))) {
        throw new Error("text node must contain a nonempty prompt of at most 7000 characters");
      }
      continue;
    }
    const image = node.type === "image_url";
    const video = node.type === "video_url";
    const audio = node.type === "audio_url";
    const imageRoles = ALLOW_FRAME_IMAGES ? ["reference_image", "first_frame", "last_frame"] : ["reference_image"];
    const validRole = image ? imageRoles.includes(node.role) :
      video ? node.role === "reference_video" : audio && node.role === "reference_audio";
    if (!validRole || Object.keys(node).some(key => !["type", "role", node.type].includes(key))) {
      throw new Error("media requires reference_image, first_frame, last_frame, reference_video or reference_audio with its matching type");
    }
    const media = node[node.type];
    if (!media || typeof media.url !== "string" || Object.keys(media).length !== 1) {
      throw new Error("media requires exactly one URL");
    }
    if (/^(asset|mm_file):\/\//.test(media.url)) {
      throw new Error("file and asset references are unsupported; use request media URLs");
    }
    if (!(/^https?:\/\//.test(media.url) ||
          (image && /^data:image\/[^,]+,.+/.test(media.url)) ||
          (video && /^data:video\/[^,]+,.+/.test(media.url)))) {
      throw new Error("media requires an HTTP(S) URL or a matching image/video Data URL");
    }
    if (image) {
      images++;
      if (node.role === "first_frame") firstFrames++;
      if (node.role === "last_frame") lastFrames++;
    } else if (video) videos++; else audios++;
  }
  if (texts !== 1 || images > MAX_IMAGES || videos > MAX_VIDEOS || audios > MAX_AUDIOS) {
    throw new Error("content requires exactly one text node, at most 9 reference images, 3 reference videos and 3 reference audios");
  }
  if (firstFrames > 1 || lastFrames > 1) {
    throw new Error("only one first_frame and one last_frame are supported");
  }
  if (firstFrames + lastFrames > 0 && (images !== firstFrames + lastFrames || videos > 0 || audios > 0)) {
    throw new Error("first/last frames cannot be combined with reference images, videos or audios");
  }

  let duration = request.duration;
  if (isMissing(duration)) {
    duration = DEFAULT_DURATION;
  }
  if (typeof duration !== "number" || !Number.isInteger(duration)) {
    throw new Error("duration must be an integer number of seconds");
  }
  if (duration < MIN_DURATION || duration > MAX_DURATION || duration > input.limits.maxDurationSeconds) {
    throw new Error("duration must be an integer from 4 to 15 seconds");
  }

  let resolution = request.resolution;
  if (isMissing(resolution)) {
    resolution = RESOLUTIONS[0];
  }
  if (typeof resolution !== "string" || !RESOLUTIONS.includes(resolution)) {
    throw new Error("resolution must be one of " + RESOLUTIONS.join(", ") + " for the selected customer model");
  }

  let ratio = request.ratio;
  if (isMissing(ratio)) {
    ratio = DEFAULT_RATIO;
  }
  if (typeof ratio !== "string" || !RATIOS.includes(ratio)) {
    throw new Error("ratio must be one of " + RATIOS.join(", ") + " for the selected customer model");
  }

  if (ratio === "adaptive" && images === 0 && videos === 0 && audios === 0) {
    throw new Error("ratio adaptive requires reference media");
  }

  let watermark = request.watermark;
  if (isMissing(watermark)) {
    watermark = WATERMARK;
  }
  if (watermark !== false) {
    throw new Error("watermark must be false for the selected customer model");
  }

  return {
    body: {
      model: providerModel,
      content: content,
      parameters: {
        duration: duration,
        resolution: resolution.toUpperCase(),
        ratio: ratio,
        prompt_optimizer: PROMPT_OPTIMIZER,
        watermark: WATERMARK,
      },
    },
  };
}

// ---------------------------------------------------------------------------
// parseCreateResponse
// ---------------------------------------------------------------------------

function parseJsonObject(value, message) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(message);
  }
  return value;
}

function parseCreateResponse(input) {
  let parsed;
  try {
    parsed = JSON.parse(input.body);
  } catch (error) {
    throw new Error("upstream create response is not valid JSON");
  }
  parsed = parseJsonObject(parsed, "upstream create response is not a JSON object");
  if (!isMissing(parsed.error)) {
    throw new Error("upstream rejected the task submission");
  }
  const result = parseJsonObject(parsed.result, "upstream create response has no result object");
  const taskID = result.task_id;
  if (typeof taskID !== "string" || taskID.trim() === "") {
    throw new Error("upstream create response has no task id");
  }
  if (result.status !== ACCEPTED_CREATE_STATUS) {
    throw new Error("upstream create response status is not a registered acceptance state");
  }
  return { id: taskID, status: result.status };
}

// ---------------------------------------------------------------------------
// parseTaskObservation
// ---------------------------------------------------------------------------

// isTrustedEmptyError accepts exactly the shapes verified not to carry a
// failure: a null/absent error, or the zero-code empty structure the verified
// success observation carries. Any other shape is a non-empty error.
function isTrustedEmptyError(value) {
  if (isMissing(value)) {
    return true;
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  if (value.code === undefined || value.code === null) {
    return value.type === undefined && value.message === undefined;
  }
  return (
    value.code === 0 &&
    (value.type === undefined || value.type === null || value.type === "") &&
    (value.message === undefined || value.message === null || value.message === "")
  );
}

function trustedErrorDetail(value) {
  if (isTrustedEmptyError(value)) {
    return null;
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  if (typeof value.code !== "number" || !Number.isInteger(value.code)) {
    return undefined;
  }
  const type = value.type;
  const message = value.message;
  if (!(type === undefined || type === null || typeof type === "string")) {
    return undefined;
  }
  if (!(message === undefined || message === null || typeof message === "string")) {
    return undefined;
  }
  return {
    code: value.code,
    type: type === undefined || type === null ? "" : type,
    message: message === undefined || message === null ? "" : message,
  };
}

// The saved JD documentation is ambiguous about whether the usage object
// sits at the top level or inside the single result item. Both verified
// shapes locate the same field; the host resolves the emitted path against
// the raw observation and validates the value itself.
function locateUsageScanRoot(parsed) {
  const topLevel = parsed.usage;
  if (topLevel !== null && typeof topLevel === "object" && !Array.isArray(topLevel)) {
    if (typeof topLevel.video_output === "number" && Number.isFinite(topLevel.video_output)) {
      return "usage.video_output";
    }
  }
  const content = parsed.content;
  if (Array.isArray(content) && content.length === 1 && content[0] !== null && typeof content[0] === "object") {
    const nested = content[0].usage;
    if (nested !== null && typeof nested === "object" && !Array.isArray(nested)) {
      if (typeof nested.video_output === "number" && Number.isFinite(nested.video_output)) {
        return "content.usage.video_output";
      }
    }
  }
  return undefined;
}

function parseTaskObservation(input) {
  let parsed;
  try {
    parsed = JSON.parse(input.body);
  } catch (error) {
    throw new Error("upstream task observation is not valid JSON");
  }
  parsed = parseJsonObject(parsed, "upstream task observation is not a JSON object");
  if (typeof parsed.task_id !== "string" || parsed.task_id === "" || parsed.task_id !== input.taskId) {
    throw new Error("task id mismatch");
  }
  const status = parsed.task_status;
  const errorDetail = trustedErrorDetail(parsed.error);
  if (errorDetail === undefined) {
    throw new Error("upstream error detail is not a trusted structure");
  }

  if (status === "pending") {
    if (errorDetail !== null) {
      throw new Error("pending status conflicts with a non-empty error");
    }
    return { id: parsed.task_id, status: "queued" };
  }
  if (status === "running") {
    if (errorDetail !== null) {
      throw new Error("running status conflicts with a non-empty error");
    }
    return { id: parsed.task_id, status: "running" };
  }
  if (status === "success") {
    if (errorDetail !== null) {
      throw new Error("success status conflicts with a non-empty error");
    }
    const content = parsed.content;
    if (!Array.isArray(content) || content.length !== 1) {
      throw new Error("success observation does not have exactly one result item");
    }
    const item = parseJsonObject(content[0], "success observation result item is not an object");
    const videoURL = item.video_url === null || typeof item.video_url !== "object" ? undefined : item.video_url.url;
    if (typeof videoURL !== "string" || !videoURL.startsWith("https://")) {
      throw new Error("success observation has no trusted video result");
    }
    const observation = { id: parsed.task_id, status: "succeeded", videoUrl: videoURL };
    const scanRoot = locateUsageScanRoot(parsed);
    if (scanRoot !== undefined) {
      observation.usageScanRoot = scanRoot;
    }
    return observation;
  }
  if (status === "failed") {
    if (errorDetail === null) {
      throw new Error("failure observation has no trusted error detail");
    }
    return {
      id: parsed.task_id,
      status: "failed",
      error: { code: String(errorDetail.code), message: errorDetail.message },
    };
  }
  if (status === "cancelled") {
    if (errorDetail !== null) {
      throw new Error("cancelled status conflicts with a non-empty error");
    }
    return { id: parsed.task_id, status: "cancelled" };
  }
  throw new Error("unsupported upstream task status");
}

export const seedance = {
  jdcloud_video_task_v1: {
    buildCreate: buildCreate,
    parseCreateResponse: parseCreateResponse,
    parseTaskObservation: parseTaskObservation,
  },
};

export const meta = {
  apiVersion: 3,
  key: "minimax-link",
  name: "MiniMax Link",
  description: {
    en: "MiniMax Link southbound protocol adapters",
    zh: "MiniMax 标准视频南向协议适配",
  },
  version: "1.2.0",
  author: { name: "yuan-gateway" },
  seedanceProtocols: ["jdcloud_video_task_v1"],
  channelConfiguration: {
    videos: [
      {
        protocol: "jdcloud_video_task_v1",
        label: "JD Cloud Video Task V1",
        models: [MODEL],
        modelMetadata: (function () {
          const metadata = {};
          metadata[MODEL] = {
            defaultDuration: DEFAULT_DURATION,
            minDuration: MIN_DURATION,
            maxDuration: MAX_DURATION,
            resolutions: RESOLUTIONS,
            ratios: RATIOS,
            maxPromptLength: MAX_PROMPT_LENGTH,
            maxImages: MAX_IMAGES,
            allowFrameImages: ALLOW_FRAME_IMAGES,
            allowVideos: true,
            maxVideos: MAX_VIDEOS,
            allowAudios: true,
            maxAudios: MAX_AUDIOS,
            deleteVideo: false,
          };
          return metadata;
        })(),
        assetProtocols: ["none"],
        defaultAssetProtocol: "none",
      },
    ],
    assets: [
      {
        protocol: "none",
        label: "No Asset Library",
        groupPolicy: "none",
        credential: "none",
      },
    ],
  },
};
