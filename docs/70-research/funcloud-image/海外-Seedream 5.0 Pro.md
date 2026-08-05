Seedream 5.0 Pro Image Generation
Overview
Seedream 5.0 Pro Image Generationendpoint, supported Text-to-Image (text-to-image) and Image-to-Image (image-to-image)Two modes.

Base URL: https://api.apiverse.ai

Authentication
All endpoints require a token in the request header for authentication:

Authorization: Bearer {YOUR_AUTH_TOKEN}
Quick Start
cURL Example
Create Text-to-Image Task

curl -X POST "https://api.apiverse.ai/api/v2/open/aigc/seedream-5.0-pro" \
 -H "Authorization: Bearer your_auth_token_here" \
 -H "Content-Type: application/json" \
 -d '{
 "prompt": "can in,",
 "genType": "t2i",
 "aspectRatio": "16:9",
 "quality": "high"
 }'
Create Image-to-Image task

curl -X POST "https://api.apiverse.ai/api/v2/open/aigc/seedream-5.0-pro" \
 -H "Authorization: Bearer your_auth_token_here" \
 -H "Content-Type: application/json" \
 -d '{
 "prompt": "will image as",
 "genType": "i2i",
 "imageUrls": ["https://example.com/reference.jpg"],
 "aspectRatio": "1:1",
 "quality": "basic"
 }'
Query Task Status

curl -X GET "https://api.apiverse.ai/api/v2/open/aigc/task_abc123" \
 -H "Authorization: Bearer your_auth_token_here"
Endpoints
1. create Seedream 5.0 Pro Image Generationtask
POST /api/v2/open/aigc/seedream-5.0-pro

Create a Seedream 5.0 Pro Image Generationtask.

Content-Type: application/json

Request Parameters
Parameter	Type	Required	Description
prompt	string	Yes	imagePrompt, 3-3000characters
genType	string	No	Generation type:t2i(Text-to-Image,default) / i2i(Image-to-Image)
imageUrls	string[]	Condition	Reference image URL (i2iwhen Required, up to 10 images, JPEG/PNG/WebP, each maximum 10MB)
aspectRatio	string	No	Aspect ratio:1:1(default) / 4:3 / 3:4 / 16:9 / 9:16 / 2:3 / 3:2
quality	string	No	output quality:basic(1K,default) / high(2K)
nsfwChecker	boolean	No	, as false when, defaultfalse
callbackUrl	string	No	Callback notification URL after the task is complete
Description:
- Image-to-Image(i2i)imageUrls is required when used
- A pre-authorization charge is applied on task creation; insufficient balance returns an error
Request Example
Text-to-Image (1Kquality):

{
 "prompt": "can in,",
 "aspectRatio": "16:9",
 "quality": "basic"
}
Text-to-Image (2Kquality):

{
 "prompt": ",",
 "genType": "t2i",
 "aspectRatio": "16:9",
 "quality": "high"
}
Image-to-Image:

{
 "prompt": "will image as, image",
 "genType": "i2i",
 "imageUrls": ["https://example.com/input.jpg"],
 "aspectRatio": "1:1",
 "quality": "basic"
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
 "taskId": "task_20260506150000_abc12345",
 "status": "processing",
 "createdAt": "2026-05-06 15:00:00"
 }
}
Insufficient balance

{
 "code": 40001,
 "msg": "Insufficient balance: first Insufficient balancetask",
 "data": null
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
 "taskId": "task_20260506150000_abc12345",
 "status": "success",
 "result": [
 "https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/2026/05/06/output_001.png"
 ],
 "createdAt": "2026-05-06 15:00:00",
 "updatedAt": "2026-05-06 15:00:30"
 }
}
3. Batch Query Task Status
POST /api/v2/open/aigc/batch

query task execute Status (up to 100 ).

4. Query Account Balance
GET /api/v2/open/balance

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
 "taskId": "task_20260506150000_abc12345",
 "status": "success",
 "result": ["https://fc-gw-sh.oss-accelerate.aliyuncs.com/images/output_001.png"],
 "errorMsg": "",
 "timestamp": "2026-05-06T15:00:30+08:00",
 "signature": "a1b2c3d4e5f6..."
}
Error Codes
code	Description
0	Success
10002	Parametermissing or malformed
10005	API Key invalid or missing
30003	Task not found
40001	Insufficient balance
90003	Internal server error
Best Practices
1. Polling Strategy
recommendedPolling:
- First 30 seconds:query every 3 seconds
- After 30 seconds:query every 5 seconds### 2. Use Callback

recommended use Callback NotificationPolling.

3. Processing Time Reference
Text-to-Image(basic/1K):Usually 5 ~ 15 seconds
Text-to-Image(high/2K):Usually 10 ~ 30 seconds
Image-to-Image(basic/1K):Usually 5 ~ 15 seconds
Image-to-Image(high/2K):Usually 10 ~ 30 seconds
4. Balance Management
Create Taskfirst recommended query
taskSuccessdeducts from the frozen balance after success
taskFailure after the frozen amount is automatically returnedto available balance