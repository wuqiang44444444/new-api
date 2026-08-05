Nano Banana 2 Image Generation
Overview
Nano Banana 2 Image Generationendpoint, supported Text-to-Image (text-to-image) and Image-to-Image editing (image-to-image), supported Resolution output (1K/2K/4K),up tocan provide 14 images reference images toImage-to-Image editing, supported Aspect ratio.

Base URL: https://api.apiverse.ai

Authentication
All endpoints require a token in the request header for authentication:

Authorization: Bearer {YOUR_AUTH_TOKEN}
Quick Start
cURL Example
Create Text-to-Image Task

curl -X POST "https://api.apiverse.ai/api/v2/open/aigc/nano-banana-2" \
 -H "Authorization: Bearer your_auth_token_here" \
 -H "Content-Type: application/json" \
 -d '{
 "prompt": "A surreal painting of a giant banana floating in space",
 "aspectRatio": "16:9",
 "resolution": "2K",
 "outputFormat": "png"
 }'
Create Image-to-Image Editing Task

curl -X POST "https://api.apiverse.ai/api/v2/open/aigc/nano-banana-2" \
 -H "Authorization: Bearer your_auth_token_here" \
 -H "Content-Type: application/json" \
 -d '{
 "prompt": "Transform the image into watercolor style",
 "imageUrls": ["https://example.com/input.jpg"],
 "aspectRatio": "1:1",
 "resolution": "2K"
 }'
Query Task Status

curl -X GET "https://api.apiverse.ai/api/v2/open/aigc/task_abc123" \
 -H "Authorization: Bearer your_auth_token_here"
Endpoints
1. Create Nano Banana 2 Image Generation Task
POST /api/v2/open/aigc/nano-banana-2

Create a Nano Banana 2 image generation task.

Content-Type: application/json

Request Parameters
Parameter	Type	Required	Description
prompt	string	Yes	imagePrompt, up to 20000characters
imageUrls	string[]	No	Reference image URL (up to 14images, JPEG/PNG/WebP, each maximum 30MB)
aspectRatio	string	No	Aspect ratio:auto(default) / 1:1 / 1:4 / 16:9 / 1:8 / 21:9 / 2:3 / 3:2 / 3:4 / 4:1 / 4:3 / 4:5 / 5:4 / 8:1 / 9:16
resolution	string	No	Output resolution:1K(default) / 2K / 4K
outputFormat	string	No	Output format:jpg(default) / png
callbackUrl	string	No	Callback notification URL after the task is complete
Description:
- When imageUrls is provided, the endpoint automatically uses image-to-image editing mode and generates from the reference images
- supported up to 14 images Reference image, each maximum 30MB, supported JPEG/PNG/WebP Format#### Request Example
Basic Text-to-Image:

{
 "prompt": "A surreal painting of a giant banana floating in space"
}
Specify Resolution and Size:

{
 "prompt": "A cute cat sitting on a windowsill, sunlight streaming in, hyper-realistic style",
 "aspectRatio": "16:9",
 "resolution": "4K",
 "outputFormat": "png"
}
Image-to-Image Editing (Single Reference):

{
 "prompt": "Transform the image into watercolor style, keep the composition",
 "imageUrls": ["https://example.com/input.jpg"],
 "aspectRatio": "1:1",
 "resolution": "2K"
}
Image-to-Image Editing (Multiple References):

{
 "prompt": "Blend the styles of these images to create a new artwork",
 "imageUrls": [
 "https://example.com/ref1.jpg",
 "https://example.com/ref2.jpg",
 "https://example.com/ref3.jpg"
 ],
 "aspectRatio": "3:2",
 "resolution": "2K"
}
With Callback URL:

{
 "prompt": "An ancient castle under the starry sky, oil painting style",
 "aspectRatio": "21:9",
 "resolution": "4K",
 "callbackUrl": "https://your-server.com/callback"
}
Response Parameters
Parameter	Type	Description
code	int	Status Code, 0 indicates success
msg	string	Message
data.taskId	string	Task ID, used to query task status
data.status	string	Task Status, fixed to processing
data.createdAt	string	Created Time
Response Example
Success

{
 "code": 0,
 "msg": "success",
 "data": {
 "taskId": "task_20260509150000_abc12345",
 "status": "processing",
 "createdAt": "2026-05-09 15:00:00"
 }
}
2. Query Task Status
GET /api/v2/open/aigc/{taskId}

query task execute Status.

Response Example
Success

{
 "code": 0,
 "msg": "success",
 "data": {
 "taskId": "task_20260509150000_abc12345",
 "status": "success",
 "result": [
 "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/09/output_001.png"
 ],
 "createdAt": "2026-05-09 15:00:00",
 "updatedAt": "2026-05-09 15:00:25"
 }
}
3. Batch Query Task Status
POST /api/v2/open/aigc/batch

query task execute Status (up to 100 ).

Callback Notification
task complete (Success or Failure) when, if Create Task when providecallbackUrl, was provided when creating the task, the system sends a POST request to that URL.

Callback Request
Headers

Content-Type: application/json
X-Funcloud-Event: task.completed
X-Funcloud-Signature: {localized example text}
Body

{
 "event": "task.completed",
 "taskId": "task_20260509150000_abc12345",
 "status": "success",
 "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
 "errorMsg": "",
 "timestamp": "2026-05-09T15:00:25+08:00",
 "signature": "a1b2c3d4e5f6..."
}
Error Codes
code	Description
0	Success
10002	Parametermissing or malformed
10005	API Key invalid or missing
30003	Task not found
90003	Internal server error
Best Practices
1. Polling Strategy
recommendedPolling:
- First 30 seconds:query every 3 seconds
- After 30 seconds:query every 5 seconds### 2. Use Callback

recommended use Callback NotificationPolling.

3. Processing Time Reference
Text-to-Image (1K):Usually 5 ~ 20 seconds
Text-to-Image (2K/4K):Usually 10 ~ 30 seconds
Image-to-Image editing:Usually 10 ~ 30 seconds
4. Resolution Selection
Resolution	Scenario
1K	,
2K	height quality, asset
4K	, dimensions