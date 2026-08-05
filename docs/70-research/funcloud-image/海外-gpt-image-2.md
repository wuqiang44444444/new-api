Image / GPT Image 2
GPT Image 2 Image Generation API (v1 / OpenAI )## Overview
MD5: 1e0a61e63d7cdb7bed07cd6def43e0a7
Copy Markdown
Copy HTML
GPT Image 2 Image Generation API (v1 / OpenAI )## Overview
localized example textOpenAI localized example textGPT Image 2 Image Generationendpoint (/v1/images/generations).

and/api/v2/open/aigc/gpt-image (:Create Task after Polling query ), endpointOpenAI localized example text:
Requestwill Image Generation complete, in in return image,no Polling.

endpointPath, Request Body, OpenAIimages/generations, use OpenAI SDK or with.

Base URL: https://api.apiverse.ai

Authentication
in Request in API Key ( and v2 endpoint):```
Authorization: Bearer {YOUR_API_KEY}


---

## Quick Start

### cURL Example

**Text-to-Image**
```bash
curl -X POST "https://api.apiverse.ai/v1/images/generations" \
-H "Authorization: Bearer your_api_key_here" \
-H "Content-Type: application/json" \
-d '{
"model": "gpt-image-2",
"prompt": "A beautiful sunset over the ocean, oil painting style",
"size": "1792x1024"
}'


**Image-to-Image**
```bash
curl -X POST "https://api.apiverse.ai/v1/images/generations" \
-H "Authorization: Bearer your_api_key_here" \
-H "Content-Type: application/json" \
-d '{
"model": "gpt-image-2",
"prompt": "Transform this image into watercolor style",
"image": "https://example.com/reference.jpg",
"size": "1024x1024"
}'


---

## endpointDescription

### create Image Generationtask ()**POST** `/v1/images/generations`

**Content-Type**: `application/json`

> **Description**:endpoint in Pollingtask complete after return.Success return image URL;> Failure return OpenAI Error.4K / etc. Scenario when can, will HTTP> when (recommended≥ 300 seconds)., will etc..

#### Request Parameters

| Parameter | Type | Required | Description |
|-----|------|-----|------|
| model | string | Yes | Model ID,:`gpt-image-2` (localized example text`gpt-image`) |
| prompt | string | Yes | imagePrompt, up to 20,000 characters|
| image | string | No | input image (URL or Base64).**fieldImage-to-Image** |
| size | string | No | output dimensions,`1024x1024`, `1792x1024`.Aspect ratio as|
| n | int | No | (OpenAI field).**endpointfield, return 1 images image**; image use`/v1/images/edits` (Image Editing)endpoint |
| quality | string | No | quality (OpenAI field)|
| response_format | string | No | Response Format:`url` (default)/ `b64_json` |
| user | string | No | (OpenAI field)|
| metadata | object | No | Parameter|

> **localized example text`size` localized example text**:will`WxH` dimensions Aspect ratio, as> `1:1` / `16:9` / `9:16` / `4:3` / `3:4` / `21:9`.localized example text`1792x1024` → `16:9`, `1024x1024` → `1:1`.
> not`size` when use Modeldefault.

> **Output resolution**:endpoint supported Resolution, use defaultResolution.
> 1K / 2K / 4K Resolution, use endpoint`/api/v2/open/aigc/gpt-image`.

#### Request Example

**Text-to-Image:**
```json
{
"model": "gpt-image-2",
"prompt": "A beautiful sunset over the ocean, oil painting style",
"size": "1792x1024"
}


**Image-to-Image:**
```json
{
"model": "gpt-image-2",
"prompt": "Transform this image into watercolor style",
"image": "https://example.com/reference.jpg",
"size": "1024x1024"
}


#### Response Parameters (Success)

| Parameter | Type | Description |
|-----|------|------|
| created | int | Created Time (Unix seconds) |
| data | array | imagelist|
| data[].url | string | image URL (`response_format=url` returned when applicable) |
| data[].b64_json | string | Base64 encodedimage (`response_format=b64_json` returned when applicable) |

#### Response Example (Success)

```json
{
"created": 1745376690,
"data": [
{
"url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/04/23/output_001.png"
}
]
}


#### ErrorError OpenAI Error:```json
{
 "error": {
 "message": "Image GenerationFailure:localized example text",
 "type": "generation_error"
 }
}
HTTP Status Code	type	Description
400	invalid_request_error	ParameterError / supported model
401	authentication_error	API Key invalid or missing
402	insufficient_quota	Insufficient balance
500	generation_error	Image GenerationFailure
500	internal_error	Internal server error
Image Editing ()POST /v1/images/edits
Content-Type: multipart/form-data

in images or multiple images image and Prompt, image / repainting /.endpointOpenAIimages/edits,
return:Polling complete after return, no Polling.

Description:and endpoint, endpoint in Polling complete return, will HTTP when (recommended≥ 300 seconds).
cURL Example
curl -X POST "https://api.apiverse.ai/v1/images/edits" \
 -H "Authorization: Bearer your_api_key_here" \
 -F "model=gpt-image-2" \
 -F "prompt=localized example text" \
 -F "size=1024x1024" \
 -F "image=@/path/to/cat.png"
multiple imagesreference image+:```bash
curl -X POST "https://api.apiverse.ai/v1/images/edits" \
-H "Authorization: Bearer your_api_key_here" \
-F "model=gpt-image-2" \
-F "prompt=images image Scenario" \
-F "n=2" \
-F "response_format=b64_json" \
-F "image=@/path/to/img1.png" \
-F "image=@/path/to/img2.png" \
-F "mask=@/path/to/mask.png"


### Request Parameters (multipart/form-data)

| Parameter | Type | Required | Description |
|-----|------|-----|------|
| model | string | Yes | Model ID,:`gpt-image-2` (localized example text`gpt-image`) |
| prompt | string | Yes | Prompt, up to 20,000 characters|
| image | file | Yes | image ().multiple imageswhen`image` fieldcan,**up to 10 images**, will|
| mask | file | No | image ().in|
| n | int | No |, range 1-10, default 1.**localized example text** |
| size | string | No | output dimensions,`1024x1024`.Aspect ratio as|
| response_format | string | No | Response Format:`url` (default)/ `b64_json` |
| user | string | No | (OpenAI field)|

### Response Parameters (Success)

| Parameter | Type | Description |
|-----|------|------|
| created | int | Created Time (Unix seconds) |
| data | array | imagelist ( etc.`n`) |
| data[].url | string | image URL (`response_format=url` returned when applicable) |
| data[].b64_json | string | Base64 encodedimage (`response_format=b64_json` returned when applicable) |

### Response Example (Success)

```json
{
"created": 1745376690,
"data": [
{
"url": "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/04/23/edit_001.png"
}
]
}


Errorand endpoint, OpenAI Error.

---

## use OpenAI SDK

endpoint OpenAI, can OpenAI SDK, only`base_url` and`api_key`:

```python
from openai import OpenAI

client = OpenAI(
api_key="your_api_key_here",
base_url="https://api.apiverse.ai/v1",
)

resp = client.images.generate(
model="gpt-image-2",
prompt="A beautiful sunset over the ocean, oil painting style",
size="1792x1024",
)
print(resp.data[0].url)


---

## Best Practices

### 1. when

This endpoint is a return.Text-to-Image / Image-to-ImageUsually 10 ~ 60 seconds, 4K or after Scenario when can,
 will when as≥ 300 seconds, first.

### 2. vs

-**(v1 endpoint)**:, Request, / and Scenario.
- **(v2`/api/v2/open/aigc/gpt-image`)**:Create Taskafter Polling or, height and / Scenario.

### 3. Prompt recommended

- use,
- can ( oil painting, watercolor, digital art etc. )
- Image-to-Image when, prompt image
- supported, Usually